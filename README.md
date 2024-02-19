# Coffee Shop Simulator

Go-based application designed to model the workflow of a coffee shop. It scales with the number of orders and gracefully handles errors and resource constraints.

## Features

### Concurrent Order Processing
- **Baristas**: A configurable number of baristas process orders in parallel.
- **Grinders and Brewers**: Multiple grinders and brewers with configurable grinding and brewing rates. Baristas access grinders and brewers concurrently via resource pools (i.e., a Go channel for each resource type to manage available grinders and brewers).
- **Order Management**: A Go channel that's used to queue customer orders to be processed by Baristas. 
- **Orders**: Supports orders with different sizes (Small, Medium, Large&mdash;with each being mapped to specific fluid ounces) and strengths.
- **Beans**: Starts with a set amount of coffee beans, with each order consuming beans based on the size and strength of the coffee.

### Real-time Interaction
- **Refill Command**: It's possible to refill coffee beans in real-time by simply entering a `refill` command in the running shell. This updates the total number of beans in the coffee shop (available to all goroutines) in a thread-safe manner using a mutex.

### Error Handling and Retries
- **Exponential Backoff**: Orders that can't be processed immediately due to unavailable resources are retried with an exponential backoff strategy. This is used in order to handle errors gracefully. 
- **Completion Tracking**: Wait-groups (`newOrdersWG`, `completedOrdersWG`) and channels (`*CoffeeShop.done`) are used to keep track of the completion status of orders and help prevent deadlocks. Failure is a valid completion state&mdash;this happens after an order fails to complete following `maxRetries` attempts.
