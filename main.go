package main

import (
	"fmt"
	"math"
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
	mu             sync.Mutex
}

type Brewer struct {
	// assume we have unlimited water, but we can only run a certain amount of water per second into our brewer + beans
	ouncesWaterPerSecond int
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
	ID             int
	size           CoffeeSize // using a more general coffee size instead of `ouncesOfCoffeeWanted`
	coffeeStrength int        // grams of beans per ounce of coffee. 2:1 for regular, 3:1 for strong etc.
}

type CoffeeShop struct {
	grindersPool             chan *Grinder
	brewersPool              chan *Brewer
	grinders                 []*Grinder
	brewers                  []*Brewer
	totalAmountUngroundBeans int
	orders                   chan Order
	done                     chan Coffee
	failedOrders             chan Order
}

func NewCoffeeShop(grinders []*Grinder, brewers []*Brewer, numCustomers int) *CoffeeShop {
	cs := &CoffeeShop{
		grindersPool:             make(chan *Grinder, len(grinders)),
		brewersPool:              make(chan *Brewer, len(brewers)),
		grinders:                 grinders,
		brewers:                  brewers,
		totalAmountUngroundBeans: 100, // default amount of coffee beans
		orders:                   make(chan Order, numCustomers),
		done:                     make(chan Coffee, numCustomers),
		failedOrders:             make(chan Order, numCustomers),
	}

	for _, g := range grinders {
		cs.grindersPool <- g
	}

	for _, b := range brewers {
		cs.brewersPool <- b
	}

	return cs
}

func (cs *CoffeeShop) MakeCoffeeOrder(order Order) {
	cs.orders <- order
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
		err := b.processOrder(order, 0)
		if err != nil {
			b.coffeeShop.failedOrders <- order
			fmt.Println(err.Error())
		}
	}
}

func (b *Barista) processOrder(order Order, retryCount int) error {
	const maxRetries = 5
	const initDelay = 3 * time.Second
	delay := time.Duration(math.Pow(2, float64(retryCount))) * initDelay // exponential backoff strategy for timeouts

	ouncesOfCoffeeWanted := coffeeSizesToOunces[order.size]
	gramsNeeded := ouncesOfCoffeeWanted * order.coffeeStrength
	ungroundBeans := Beans{weightGrams: gramsNeeded, state: Unground}

	var groundBeans Beans
	var grinder *Grinder
	select {
	case grinder = <-b.coffeeShop.grindersPool:
	case <-time.After(delay):
		if retryCount < maxRetries {
			fmt.Printf(Format(BLUE, fmt.Sprintf("Barista %v retrying order %v, attempt %v\n", b.ID, order.ID, retryCount+1)))
			return b.processOrder(order, retryCount+1)
		} else {
			return fmt.Errorf(Format(RED, fmt.Sprintf("Barista %v failed to process order %v after %v attempts", b.ID, order.ID, retryCount)))
		}
	}
	retryCount = 0 // reset retryCount for Brewer

	groundBeans = grinder.Grind(ungroundBeans)
	b.coffeeShop.grindersPool <- grinder
	groundBeans.state = Ground

	var coffee Coffee
	var brewer *Brewer
	select {
	case brewer = <-b.coffeeShop.brewersPool:
	case <-time.After(delay):
		if retryCount < maxRetries {
			fmt.Printf(Format(BLUE, fmt.Sprintf("Barista %v retrying order %v, attempt %v\n", b.ID, order.ID, retryCount+1)))
			return b.processOrder(order, retryCount+1)
		} else {
			return fmt.Errorf(Format(RED, fmt.Sprintf("Barista %v failed to process order %v after %v attempts", b.ID, order.ID, retryCount)))
		}
	}
	coffee = brewer.Brew(groundBeans, order.size, order.ID)
	b.coffeeShop.brewersPool <- brewer
	groundBeans.state = Brewed

	b.coffeeShop.done <- coffee
	return nil
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

	brewTime := (beans.weightGrams / 2) * 1 / b.ouncesWaterPerSecond
	fmt.Printf(Format(PURPLE, fmt.Sprintf("Waiting %v seconds to brew order %v: %v\n", time.Duration(brewTime)*time.Second, orderID, size)))
	time.Sleep(time.Duration(brewTime) * time.Second)
	return Coffee{
		orderID,
		size,
	}
}

func main() {
	// Premise: we want to model a coffee shop. An order comes in, and then with a limited amount of grinders and
	// brewers (each of which can be "busy"): we must grind unground beans, take the resulting ground beans, and then
	// brew them into liquid coffee. We need to coordinate the work when grinders and/or brewers are busy doing work
	// already. What Go datastructure(s) might help us coordinate the steps: order -> grinder -> brewer -> coffee?
	//
	// Some of the struct types and their functions need to be filled in properly. It may be helpful to finish the
	// Grinder impl, and then Brewer impl each, and then see how things all fit together inside CoffeeShop afterwards.
	numCustomers := 5
	var wg sync.WaitGroup
	wg.Add(numCustomers)

	//b := Beans{weightGrams: 10}
	g1 := &Grinder{gramsPerSecond: 5}
	g2 := &Grinder{gramsPerSecond: 3}
	g3 := &Grinder{gramsPerSecond: 12}

	b1 := &Brewer{ouncesWaterPerSecond: 10}
	b2 := &Brewer{ouncesWaterPerSecond: 5}

	cs := NewCoffeeShop([]*Grinder{g1, g2, g3}, []*Brewer{b1, b2}, numCustomers)
	go cs.StartBaristas(100)

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
			time.Sleep(100 * time.Millisecond) // slight delay between creating each order
		}()
	}
	wg.Wait()
	close(cs.orders)

	// Wait for all coffees to be done
	for i := 0; i < numCustomers; i++ {
		c := <-cs.done
		fmt.Printf(Format(GREEN, fmt.Sprintf("Order %d completed\n", c.OrderID)))
	}
	close(cs.done)

	for j := 0; j < len(cs.failedOrders); j++ {
		<-cs.failedOrders
	}
	close(cs.failedOrders)

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
