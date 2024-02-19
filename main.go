package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type BeanState int

const (
	Unground BeanState = iota
	Ground
	Brewed
)

type BeanType int

const (
	Arabica BeanType = iota
	Robusta
	Excelsa
	Liberica
)

type Beans struct {
	// indicate some state change? create a new type?
	weightGrams int
	state       BeanState
	beanType    BeanType
}

type Grinder struct {
	gramsPerSecond int
	busy           bool
	mu             sync.Mutex
}

type Brewer struct {
	// assume we have unlimited water, but we can only run a certain amount of water per second into our brewer + beans
	ouncesWaterPerSecond int
	busy                 bool
	mu                   sync.Mutex
}

type CoffeeSize int

const (
	Small CoffeeSize = iota
	Medium
	Large
)

var coffeeSizesToOunces = map[CoffeeSize]int{
	Small:  4,
	Medium: 6,
	Large:  12,
}

type Order struct {
	ID   int
	size CoffeeSize
	//ouncesOfCoffeeWanted int
	coffeeStrength int // grams of beans per ounce of coffee. 2:1 for regular, 3:1 for strong etc.
}

type CoffeeShop struct {
	grinders                 []*Grinder
	brewers                  []*Brewer
	totalAmountUngroundBeans int
	orders                   chan Order
	done                     chan Coffee
}

type Coffee struct {
	// should hold size maybe?
	OrderID int
	size    CoffeeSize
}

type Barista struct {
	ID         int
	coffeeShop *CoffeeShop
}

func NewBarista(id int, coffeeShop *CoffeeShop) *Barista {
	return &Barista{
		ID:         id,
		coffeeShop: coffeeShop,
	}
}

func (b *Barista) ProcessOrders() {
	for order := range b.coffeeShop.orders {
		fmt.Printf(Format(YELLOW, fmt.Sprintf("Barista %d processing order %d\n", b.ID, order.ID)))
		b.processOrder(order)
		fmt.Printf(Format(GREEN, fmt.Sprintf("Order %d completed by Barista %d\n", order.ID, b.ID)))
	}
}

func (b *Barista) processOrder(order Order) {
	ouncesOfCoffeeWanted := coffeeSizesToOunces[order.size]
	gramsNeeded := ouncesOfCoffeeWanted * order.coffeeStrength

	ungroundBeans := Beans{weightGrams: gramsNeeded, state: Unground}

	var groundBeans Beans
	for {
		grinderFound := false
		for _, grinder := range b.coffeeShop.grinders {
			grinder.mu.Lock()
			if !grinder.busy {
				grinder.busy = true
				grinder.mu.Unlock()
				groundBeans = grinder.Grind(ungroundBeans)
				grinderFound = true
				break
			}
			grinder.mu.Unlock()
		}

		if grinderFound {
			break
		} else {
			err := fmt.Errorf("no available grinder for order %d, retrying ...", order.ID)
			fmt.Println(Format(RED, err.Error()))
			time.Sleep(500 * time.Millisecond)
		}
	}

	for {
		brewerFound := false
		for _, brewer := range b.coffeeShop.brewers {
			brewer.mu.Lock()
			if !brewer.busy {
				brewer.busy = true
				brewer.mu.Unlock()
				groundBeans.state = Brewed
				b.coffeeShop.done <- brewer.Brew(groundBeans, order.size, order.ID)
				brewerFound = true
				break
			}
			brewer.mu.Unlock()
		}

		if brewerFound {
			break
		} else {
			err := fmt.Errorf("no available brewer for order %d, retrying ...", order.ID)
			fmt.Println(Format(RED, err.Error()))
			time.Sleep(500 * time.Millisecond)
		}
	}

}

func (cs *CoffeeShop) StartBaristas(numBaristas int) {
	for i := 1; i <= numBaristas; i++ {
		barista := NewBarista(i, cs)
		go barista.ProcessOrders()
	}
}

func (g *Grinder) Grind(beans Beans) Beans {
	// how long should it take this function to complete?
	// i.e. time.Sleep(XXX)
	g.mu.Lock()
	defer g.mu.Unlock()

	g.busy = true
	defer func() { g.busy = false }()

	grindTime := beans.weightGrams / g.gramsPerSecond
	time.Sleep(time.Duration(grindTime) * time.Second)
	return Beans{
		weightGrams: beans.weightGrams,
		state:       Ground,
		beanType:    beans.beanType,
	}
}

func (b *Brewer) Brew(beans Beans, size CoffeeSize, orderID int) Coffee {
	// assume we need 6 ounces of water for every 12 grams of beans
	// how long should it take this function to complete?
	// i.e. time.Sleep(YYY)
	b.mu.Lock()
	defer b.mu.Unlock()

	b.busy = true
	defer func() { b.busy = false }()

	brewTime := (beans.weightGrams / 2) * 1 / b.ouncesWaterPerSecond
	fmt.Printf(Format(PURPLE, fmt.Sprintf("Waiting %v seconds to brew order %v: %v\n", time.Duration(brewTime)*time.Second, orderID, size)))
	time.Sleep(time.Duration(brewTime) * time.Second)
	return Coffee{
		orderID,
		size,
	}
}

func NewCoffeeShop(grinders []*Grinder, brewers []*Brewer) *CoffeeShop {
	return &CoffeeShop{
		grinders:                 grinders,
		brewers:                  brewers,
		totalAmountUngroundBeans: 100, // default amount of coffee beans
		orders:                   make(chan Order, 10),
		done:                     make(chan Coffee, 10),
	}
}

func (cs *CoffeeShop) MakeCoffeeOrder(order Order) {
	cs.orders <- order
}

func main() {
	// Premise: we want to model a coffee shop. An order comes in, and then with a limited amount of grinders and
	// brewers (each of which can be "busy"): we must grind unground beans, take the resulting ground beans, and then
	// brew them into liquid coffee. We need to coordinate the work when grinders and/or brewers are busy doing work
	// already. What Go datastructure(s) might help us coordinate the steps: order -> grinder -> brewer -> coffee?
	//
	// Some of the struct types and their functions need to be filled in properly. It may be helpful to finish the
	// Grinder impl, and then Brewer impl each, and then see how things all fit together inside CoffeeShop afterwards.
	numCustomers := 10
	var wg sync.WaitGroup
	wg.Add(numCustomers)

	//b := Beans{weightGrams: 10}
	g1 := &Grinder{gramsPerSecond: 5}
	g2 := &Grinder{gramsPerSecond: 3}
	g3 := &Grinder{gramsPerSecond: 12}

	b1 := &Brewer{ouncesWaterPerSecond: 2}
	b2 := &Brewer{ouncesWaterPerSecond: 5}

	cs := NewCoffeeShop([]*Grinder{g1, g2, g3}, []*Brewer{b1, b2})

	go cs.StartBaristas(5)

	sizes := []CoffeeSize{Small, Medium, Large}
	for i := 0; i < numCustomers; i++ {
		// in parallel, all at once, make calls to MakeCoffeeOrder
		i := i
		go func() {
			defer wg.Done()
			size := sizes[rand.Intn(len(sizes))]
			cs.MakeCoffeeOrder(
				Order{
					ID:             i,
					size:           size,
					coffeeStrength: rand.Intn(3) + 1,
				},
			)
			fmt.Printf("Customer %d served %v \n", i, size)
			time.Sleep(100 * time.Millisecond)
		}()
	}
	wg.Wait()

	close(cs.orders)

	// Wait for all coffees to be done
	for i := 0; i < numCustomers; i++ {
		<-cs.done
	}
	close(cs.done)

	// Issues with the above
	// 1. Assumes that we have unlimited amounts of grinders and brewers.
	//		- How do we build in logic that takes into account that a given Grinder or Brewer is busy?
	// 2. Does not take into account that brewers must be used after grinders are done.
	// 		- Making a coffee needs to be done sequentially: find an open grinder, grind the beans, find an open brewer,
	//		  brew the ground beans into coffee.
	// 3. A lot of assumptions (i.e. 2 grams needed for 1 ounce of coffee) are left as comments in the code.
	// 		- How can we make these assumptions configurable, so that our coffee shop can serve let's say different
	//		  strengths of coffee via the Order that is placed (i.e. 5 grams of beans to make 1 ounce of coffee)?
}
