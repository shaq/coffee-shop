package main

import (
	"bufio"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sync"
	"time"
)

var (
	completedOrdersWG sync.WaitGroup
	newOrdersWG       sync.WaitGroup
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
	weightGrams int
	state       BeanState
	beanType    BeanType
}

type Grinder struct {
	gramsPerSecond int
}

func (g *Grinder) Grind(beans Beans) Beans {
	grindTime := beans.weightGrams / g.gramsPerSecond
	time.Sleep(time.Duration(grindTime) * time.Second)
	return Beans{
		weightGrams: beans.weightGrams,
		state:       Ground,
		beanType:    beans.beanType,
	}
}

type Brewer struct {
	// assume we have unlimited water, but we can only run a certain amount of water per second into our brewer + beans
	ouncesWaterPerSecond int
}

func (b *Brewer) Brew(beans Beans, size CoffeeSize, orderID int) Coffee {
	// assume we need 6 ounces of water for every 12 grams of beans
	brewTime := (beans.weightGrams / 2) * 1 / b.ouncesWaterPerSecond
	fmt.Printf(Format(PURPLE, fmt.Sprintf("Waiting %v seconds to brew order %v: %v with %v beans\n", time.Duration(brewTime)*time.Second, orderID, size, beans.beanType)))
	time.Sleep(time.Duration(brewTime) * time.Second)
	return Coffee{
		orderID: orderID,
		size:    size,
		status:  Success,
	}
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

type CoffeeStatus int

const (
	Success CoffeeStatus = iota
	Failure
)

type Coffee struct {
	// should hold size maybe?
	orderID int
	size    CoffeeSize
	status  CoffeeStatus
}

type CoffeeShop struct {
	grindersPool             chan *Grinder
	brewersPool              chan *Brewer
	totalAmountUngroundBeans int
	beansMutex               sync.Mutex
	orders                   chan Order
	done                     chan Coffee
	refills                  chan struct{}
}

func NewCoffeeShop(grinders []*Grinder, brewers []*Brewer, numCustomers int, ungroundBeans int) *CoffeeShop {
	cs := &CoffeeShop{
		grindersPool:             make(chan *Grinder, len(grinders)),
		brewersPool:              make(chan *Brewer, len(brewers)),
		totalAmountUngroundBeans: ungroundBeans,
		orders:                   make(chan Order, numCustomers),
		done:                     make(chan Coffee, numCustomers),
		refills:                  make(chan struct{}, 1),
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
	newOrdersWG.Done()
}

func (cs *CoffeeShop) RefillBeans(amount int) {
	cs.beansMutex.Lock()
	defer cs.beansMutex.Unlock()

	cs.totalAmountUngroundBeans += amount
	fmt.Printf("Beans have been refilled! Total amount now: %v\n", cs.totalAmountUngroundBeans)
	cs.refills <- struct{}{} // signal that a refill has occurred
}

func (cs *CoffeeShop) StartBaristas(numBaristas int) {
	for i := 1; i <= numBaristas; i++ {
		barista := NewBarista(i, cs)
		go barista.ProcessOrders()
	}
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

func (b *Barista) UseBeans(amount int) bool {
	b.coffeeShop.beansMutex.Lock()
	defer b.coffeeShop.beansMutex.Unlock()

	if b.coffeeShop.totalAmountUngroundBeans < amount {
		return false
	}

	b.coffeeShop.totalAmountUngroundBeans -= amount
	return true
}

func (b *Barista) ReturnBeans(amount int) {
	b.coffeeShop.beansMutex.Lock()
	b.coffeeShop.totalAmountUngroundBeans += amount
	b.coffeeShop.beansMutex.Unlock()
}

func (b *Barista) ProcessOrders() {
	for order := range b.coffeeShop.orders {
		fmt.Printf(Format(YELLOW, fmt.Sprintf("Barista %d processing order %d\n", b.ID, order.ID)))
		err := b.processOrder(order, 0, Beans{})
		if err != nil {
			fmt.Println(err.Error())
		}
	}
}

// chose not to make this function DRY to increase readability of complex control flow
func (b *Barista) processOrder(order Order, retryCount int, beans Beans) error {
	const maxRetries = 5
	const initDelay = 500 * time.Millisecond
	// exponential backoff for retries
	delay := time.Duration(math.Pow(2, float64(retryCount))) * initDelay

	// calculate the amount of beans needed to make the order
	ouncesOfCoffeeWanted := coffeeSizesToOunces[order.size]
	gramsNeeded := ouncesOfCoffeeWanted * order.coffeeStrength
	if beans == (Beans{}) {
		beans = Beans{weightGrams: gramsNeeded, state: Unground}
	}

	// Barista first checks if there are enough beans to process the order
	if !b.UseBeans(gramsNeeded) {
		// retry if not
		if retryCount < maxRetries {
			fmt.Printf(Format(RED, fmt.Sprintf("Not enough beans for order %v: %v with %v beans\n", order.ID, order.size, beans.beanType)))
			fmt.Printf(Format(BLUE, fmt.Sprintf("Barista %v retrying order %v, attempt %v\n", b.ID, order.ID, retryCount+1)))
			time.Sleep(delay)
			return b.processOrder(order, retryCount+1, Beans{})
		} else {
			// if after `maxRetries` attempts there are still not enough beans, mark order as processed
			b.markOrderProcessed(Coffee{
				orderID: order.ID,
				status:  Failure,
			})
			return fmt.Errorf(Format(RED, fmt.Sprintf("Barista %v failed to process order %v after %v attempts: not enough beans.", b.ID, order.ID, retryCount)))
		}
	}

	//var groundBeans Beans
	var grinder *Grinder
	// only attempt to grind beans if we haven't successfully before
	if beans.state != Ground {
		select {
		// try to acquire grinder from pool
		case grinder = <-b.coffeeShop.grindersPool:
			beans = grinder.Grind(beans)
			beans.state = Ground
			b.coffeeShop.grindersPool <- grinder
		case <-time.After(delay):
			if retryCount < maxRetries {
				b.ReturnBeans(gramsNeeded)
				fmt.Printf(Format(BLUE, fmt.Sprintf("Grind: Barista %v retrying order %v, attempt %v\n", b.ID, order.ID, retryCount+1)))
				return b.processOrder(order, retryCount+1, Beans{})
			} else {
				// if after `maxRetries` attempts there are still no grinders available, return beans and mark order as processed
				b.ReturnBeans(gramsNeeded)
				b.markOrderProcessed(Coffee{
					orderID: order.ID,
					status:  Failure,
				})
				return fmt.Errorf(Format(RED, fmt.Sprintf("Barista %v failed to process order %v after %v attempts: grinder error.", b.ID, order.ID, retryCount)))
			}
		case <-b.coffeeShop.refills:
			// if beans are refilled, try the order again and reset retries
			return b.processOrder(order, 0, Beans{})
		}
		// reset retries for brewing
		retryCount = 0
	}

	var coffee Coffee
	var brewer *Brewer
	select {
	case brewer = <-b.coffeeShop.brewersPool:
		coffee = brewer.Brew(beans, order.size, order.ID)
		beans.state = Brewed
		b.coffeeShop.brewersPool <- brewer
		b.markOrderProcessed(coffee)
		return nil
	case <-time.After(delay):
		if retryCount < maxRetries {
			fmt.Printf(Format(BLUE, fmt.Sprintf("Brew: Barista %v retrying order %v, attempt %v\n", b.ID, order.ID, retryCount+1)))
			return b.processOrder(order, retryCount+1, beans)
		} else {
			// if after `maxRetries` attempts there are still no brewers available, just mark order as processed.
			// (can't return beans that have already been ground)
			b.markOrderProcessed(Coffee{
				orderID: order.ID,
				status:  Failure,
			})
			return fmt.Errorf(Format(RED, fmt.Sprintf("Barista %v failed to process order %v after %v attempts: brewer error.", b.ID, order.ID, retryCount)))
		}
	}
}

func (b *Barista) markOrderProcessed(coffee Coffee) {
	b.coffeeShop.done <- coffee
	completedOrdersWG.Done()
}

func main() {
	numCustomers := 10
	newOrdersWG.Add(numCustomers)
	completedOrdersWG.Add(numCustomers)

	beanFill := 500
	g1 := &Grinder{gramsPerSecond: 5}
	g2 := &Grinder{gramsPerSecond: 3}
	g3 := &Grinder{gramsPerSecond: 12}

	b1 := &Brewer{ouncesWaterPerSecond: 10}
	b2 := &Brewer{ouncesWaterPerSecond: 5}

	cs := NewCoffeeShop([]*Grinder{g1, g2, g3}, []*Brewer{b1, b2}, numCustomers, beanFill)
	go cs.StartBaristas(2)

	// listen for "refill" command
	commandChan := make(chan string)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			commandChan <- scanner.Text()
		}
	}()

	go func() {
		for cmd := range commandChan {
			if cmd == "refill" {
				cs.RefillBeans(beanFill)
			}
		}
	}()

	sizes := []CoffeeSize{Small, Medium, Large}
	for i := 0; i < numCustomers; i++ {
		// in parallel, all at once, make calls to MakeCoffeeOrder to create new orders
		i := i
		go func() {
			size := sizes[rand.Intn(len(sizes))]
			cs.MakeCoffeeOrder(
				Order{
					ID:             i,
					size:           size,
					coffeeStrength: rand.Intn(3) + 1,
				},
			)
			fmt.Printf("Customer %d served %v \n", i, size)
		}()
		// slight delay between creating each order
		time.Sleep(100 * time.Millisecond)
	}
	newOrdersWG.Wait()
	close(cs.orders)

	// wait for all orders to be processed then close channel
	go func() {
		completedOrdersWG.Wait()
		close(cs.done)
	}()

	// simultaneously read from done channel
	for coffee := range cs.done {
		if coffee.status == Success {
			fmt.Printf(Format(GREEN, fmt.Sprintf("Order %d completed successfully! %v\n", coffee.orderID, coffee.size)))
		}
	}
}
