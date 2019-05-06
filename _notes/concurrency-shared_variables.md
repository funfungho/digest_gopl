# Race conditions
- When we cannot confidently say that one event happens before the other, then the events `x` and `y` are concurrent
- A function is concurrency-safe if it continues to work correctly even when called concurrently, that is, from 2 or more goroutines with no additional synchronization
- A type is concurrency-safe if all its accessible methods and operations are concurrency-safe
- We can make a program concurrency-safe without making every concrete type in that program concurrency-safe
    - Concurrency-safe types are the exceptions rather than the rule
    - You should access a variable concurrently only if the documentation for its type says that it's safe
- We avoid concurrent access to most variables either by confining them to a single goroutine or by maintaining a higher-level invariant of mutual exclusion
- Exported package-level functions are generally expected to be concurrency-safe
    - Since package-level variables cannot be confined to a single goroutine, functions that modify them must enforce mutual exclusion
- Many reasons a function might not work when called concurrently, including dead-lock, livelock, and resource starvation
- A race condition is a situation in which the program does not give the correct result for some interleavings of the operations of multiple goroutines
- Race conditions are pernicious because they may remain **latent** in a program and appear infrequently, perhaps only under heavy load or when certain compilers, platforms, or architectures
- A data race occurs whenever 2 goroutines access the same variable concurrently and at least of one the accesses is a **write**

    ```go
    package bank
    var balance int
    // 2 operation
    // read: balance + amount
    // write: balance = ...
    func Deposit(amount int) { balance = balance + amount}
    func Balance() int { return balance }

    go func() {
        bank.Deposit(200)
        fmt.Println("=", balance.Balance())
    }()
    go bank.Deposit(100)
    ```

    - If the second deposit occurs in the middle of the first deposit, after the balance has been read but before it has been updated, it'll cause the second transaction to [ ] disappear
- Things get messier if the data race involves a variable of a type that is larger than a single machine word

    ```go
    var x []int
    go func() { x = make([]int, 10) }()
    go func() { x = make([]int, 1000000) }()
    x[999999] = 1
    ```

    - `x` could be nil, or a slice of length, or a slice of length 1000000
    - There're 3 parts to a slice: the pointer, the length, and the capacity
    - If the pointer comes from the first call to `make` and the length comes from the second, `x` would be a slice whose nominal length is 1000000 but whose underlying array has only 10 elements. In this eventuality, storing a element 999999 would clobber an arbitrary faraway location, with consequences that are impossible to predict and hard to debug and localize
    - The semantic minefield is called undefined behavior
- The notion that a concurrent program is an interleaving of several sequential programs is a false intuition
- A good rule of thumb is there's no such thing as a benign data race
- 3 ways to avoid a data race
    1. Not to write variable
    2. Avoid accessing the variable from multiple goroutines (variable confined to a single goroutine)
        - Since other goroutines cannot access the variable directly, they must use a channel to send the confining goroutine a request to query or update the variable
        - This is what's meant by the Go mantra "Do not communicate by sharing memory; instead, share memory by communicating"
        - A goroutine that brokers access to a confined variable using channel requests is called a **monitor goroutine** for that variable
    3. Allow may goroutines to access the variable, but only one at a time (mutual exclusion)
- It's common to share a variable between goroutines in a pipeline by passing its address from one stage to the next over a channel. If each stage of the pipeline refrains from access the variable after sending it to the next stage, then all accesses to the variable are sequential. In effect, the variable is confined to one stage of the pipeline, then confined to the next, and so on
    - This discipline is sometimes called *serial confinement*

        ```go
        type Cake struct{ state string }
        func baker(cooked chan<- *Cake) {
            for {
                cake := new(Cake)
                cake.state = "cooked"
                cooked <- cake // baker never touches this cake again
            }
        }
        func icer(iced chan<- *Cake, cooked <-chan *Cake) {
            for cake := range cooked {
                cake.state = "iced"
                iced <- cake // icer never touches this cake again
            }
        }
        ```

# Mutual exclusion: `sync.Mutex`
- Use a channel of capacity of 1 to ensure that at most 1 goroutine accesses a shared variable at a time
- A semaphore that counts only to 1 is called a **binary semaphore**
- This pattern of mutual exclusion is so useful that it's supported directly by the `Mutex` type from `sync` package
    - Its `Lock` method acquires the token (called a lock) and its `Unlock` method releases it
    - Each time a goroutine accesses the shared variables, it must call the mutex's `Lock` method to **acquire a exclusive lock**. If some other goroutine has acquire the lock, this operation will **block** util the other goroutine calls `Unlock` and the lock becomes available again
    - The mutex guards the shared variables
    - By convention, the variables guarded by a mutex are declared immediately after the declaration of the mutex itself
        - If you deviate from this, be sure to document it
- The region of code between `Lock` and `Unlock` in which a goroutine is free to read and modify the shared variables is called a critical section
- It's essential that the goroutine release the lock **once** it is finish, **on all paths** through the function, including error path
    - By deferring a call to `Unlock`, the critical section **implicitly extends to the end of the current function**, freeing us from having to remember to insert `Unlock` calls in one or more places far from the the call to `Lock`
    - A deferred `Unlock` will run even if the critical section panics, which may be important in programs that make use of `recover`
- A `defer` is marginally more expensive than an explicit call to `Unlock`, but not expensive enough to justify less clear code
    - As with concurrent programs, favor clarity and resist premature optimization
    - Where possible, use `defer` and let critical sections to extend to the end of a function
- Mutex locks are **not re-entrant** - it's not possible to lock a mutex that's already locked - this leads to a deadlock where nothing can proceed
    - When a goroutine acquires mutex lock, it may assume that the invariants of the shared variables hold
    - While it holds the lock, it may update the shared variables so that the invariants are temporarily violated
    - When it releases the lock, it must guarantee that the invariants hold again
- A common solution (to achieve atomic) is to divide a function into 2: an unexported function that assumes the `Lock` is already held and does the real work, and an exported function that acquires the lock before calling the unexported one

    ```go
    // not atomic
    func Withdraw(amount int) bool {
        Deposit(-amount)
        if Balance() < 0 {
            Deposit(amount)
            return false // insufficient funds
        }
        return true
    }
    // When an excessive withdrawal is attempted, the balance transiently 
    // dips below zero. This may causes a concurrent withdrawal for a modest 
    // sum to be spuriously rejected
    ```

- Encapsulation by reducing unexpected interactions in a program helps to maintain data structure invariants
- For the same reason, encapsulation also helps to maintain concurrency invariants
- When using a mutex, make sure that both it and the variables it guards are not exported, whether they are package-level variables or fields of a struct
# Read/Write mutexes: `sync.RWMutex`
- `sync.RWMutex` - a multiple readers, single writer lock
    - A special kind of lock that allows read-only operations to proceed in parallel with each other, but write operations to have fully exclusive access

    ```go
    var mu sync.RWMutex
    var balance int
    func Balance() int {
        mu.RLock()
        defer mu.RUnlock()
        return balance
    }
    ```

- `Rlock` can be used only if there are no writes to shared variables in the critical section
    - In general, we should not assume that logically read-only functions or methods don't also update some variables
        - A method that appears to be a simple accessor might also increment an internal usage counter, or update a cache so that repeat calls are faster
    - If in doubt, use an exclusive lock
- It's only profitable to use an `RWMutex` when most of the goroutine that acquire the lock are readers, and the lock is under contention, that is, goroutines routinely have to wait to acquire it
- An `RWMutex` requires more complex internal bookkeeping, making it slower than a regular mutex for uncontended locks
# Memory synchronization
- Unlike `Deposit`, `Balance` consists only a single operation, so there's no danger of another goroutine executing "in the middle" of it
- The reason `Balance` needs mutual exclusion, either channel-based or mutex-based
    1. It's equally important that `Balance` not execute in the middle of some other operation like `Withdraw`
    2. Synchronization is about more than just the order of execution of multiple goroutines; synchronization also affects memory
- In a modern computer, there may be dozens of processors, each with its own local cache of the main memory
- For efficiency, writes to memory are buffered within each processor and flushed out to main memory when necessary
- They may even be committed to main memory in a different order than they were written by the writing goroutine
- Synchronization primitives like channel communications and mutex operations cause the processor to flush out and commit **all its accumulated writes** so that the effects of goroutine execution up to that point are guaranteed to be visible to goroutines running on other processors

    ```go
    var x, y int
    go func() {
        x = 1                   // A1
        fmt.Print("y:", y, " ") // A2
    }()
    go func() {
        y = 1                   // B1
        fmt.Print("x:", x, " ") // B2
    }()
    // these 2 outcomes might come as a surprise
    // x:0 y:0
    // y:0 x:0
    ```

    - Depending on the compiler, CPU, and many other factors, they can happen
    - Although goroutine A must observe the effect of the write `x=1` before it reads the value of `y`, it does not necessarily observe the write to `y` done by goroutine B
- **Within a single goroutine**, the effects of each statement are guaranteed to occur in the order of execution; goroutines are **sequentially consistent**
- But in the absence of explicit synchronization using a channel or mutex, there's no guarantee that events are seen in the same order by all goroutines
- It's tempting to try to understand concurrency as if it corresponds to some interleaving of the statements of each goroutine, but this is not how a modern compiler or CPU works
    - Because the assignment and the `Print` refer to different variables, a compiler may conclude that the order of these 2 statements **cannot affect the result**, and **swap** them
    - If the 2 goroutines execute on different CPUs, each with its own cache, writes by one goroutine are not visible to the other goroutine's `Print` until the caches are synchronized with main memory
- All these concurrency problems can be avoided by the consistent use of simple, established patterns
    - Where possible, confine variables to a single goroutine; for other variables, use mutual exclusion
# Lazy initialization: `sync.Once`
- It's a good practice to defer an expensive initialization step until the moment it's needed
- Initializing a variable up front increases the start-up latency of a program and is unnecessary if execution doesn't always reach the part of the program that uses that variable
- In the absence of explicit synchronization, the compiler and CPU are free to **reorder** accesses to memory in any number of ways, so long as the behavior of each goroutine itself is sequentially consistent
- Example

    ```go
    var icon map[string]image.Image
    func loadIcons() {
        icons = map[string]image.Image{
            "spades.png": loadIcon("spades.png"),
            "hearts.png": loadIcon("hearts.png"),
            "diamonds.png": loadIcon("diamonds.png"),
            "clubs.png": loadIcon("clubs.png"),
        }
    }
    // Not concurrency-safe
    func Icon(name string) image.Image {
        if icons == nil {
            loadIcons()
        }
        return icons[name]
    }
    ``` 

    - > Possible reorder

        ```go
        var icon map[string]image.Image
        func loadIcons() {
            icons = make(map[string]image.Image)
            icons["spades.png"] = loadIcon("spades.png")
            icons["hearts.png"] = loadIcon("hearts.png")
            icons["diamonds.png"] = loadIcon("diamonds.png")
            icons["clubs.png"] = loadIcon("clubs.png")
        }
        ```

        - Consequently, a goroutine finding `icons` to be non-nil should (can) not assume that the initialization of the variable is complete (可能只初始化了部分图片)
    - Using a mutex

        ```go
        var mu sync.Mutex // guards icons
        var icons map[string]image.Image
        func Icon(name string) image.Image {
            mu.Lock()
            defer mu.Unlock()
            if (icons == nil) {
                loadIcons()
            }
            return icons[name]
        }
        ```

        - The cost of enforcing mutually exclusive access to `icons` is that 2 goroutines cannot access the variable concurrently, even once the variable has been safely initialized and will never be modified again
    - Using a multiple-readers lock

        ```go
        var mu sync.Mutex // guards icons
        var icons map[string]image.Image
        func Icon(name string) image.Image {
            mu.RLock()
            if icons != nil {
                icon := icons[name]
                mu.RUnlock()
                return icon
            }
            mu.RUnlock()

            mu.Lock()
            if icon == nil { // must recheck for nil
                loadIcons()
            }
            icon := icons[name]
            mu.Unlock()
            return icon
        }
        ```
        - There's no way to upgrade a shared lock to an exclusive one without first releasing the shared lock
        - Must recheck the `icons` in case another goroutine already initialized it in the interim
        - This pattern gives us greater concurrency but is complex and thus error-prone
- The `sync` package provides a specialized solution to the problem of one-time initialization: `sync.Once`
    - Conceptually, a `Once` consists of a mutex and a boolean variable variable that records whether initialization has taken place
    - The mutex guards both the boolean and the client's data structures
    - The sole method, `Do`, accepts the initialization function as its argument

        ```go
        var loadIconOnce sync.Once // guards icons
        var icons map[string]image.Image
        func Icon(name string) image.Image {
            loadIconOnce.Do(loadIcons)
            return icons[names]
        }
        ```

        - Each call to `Do(loadIcons)` locks the mutex and check the boolean value
        - In the first call in which the variable is false, `Do` calls `loadIcons` and sets the variables to true
        - Subsequent calls do nothing, but mutex synchronization ensures that the effects of `loadIcons` on memory (specially, `icons`) become visible to all goroutine
- Using `sync.Once` this way, we **can avoid sharing variables with other goroutines until they have been properly constructed**
# The race detector
- Go runtime and toolchain are equipped with a sophisticated and easy-to-use dynamic analysis tool - the race detector
- Add `-race` flag to `go build`, `go run`, or `go test`
    - This causes the compiler to build a modified version of the application or test with additional instrumentation that effectively records all accesses to shared variables that  occurred execution, along with the identity of the goroutine that read or wrote the variable
    - In addition, the modified program records all synchronization events, such as `go` statements, channel operations, and calls to `(*sync.Mutex).Lock`, `(*sync.WaitGrout).Wait`, and so on
    - > The complete set of synchronization events is specified by the *The Go Memory Model* document that accompanies the language specification
- The race detector studies this stream of events, looking for cases in which one goroutine reads or writes a shared variable that was most recently written by a different goroutine **without an intervening synchronization operation**
    - This indicates a concurrent access to the shared variable, and thus a data race
- The tool prints a report that includes the identity of the variable, and the stacks of active function calls in the reading goroutine and the writing goroutine
- The race detector reports all data races that were actually executed. However, it can only detect race conditions that occur during a run; it cannot prove that none will ever occur. For best results, make sure that your tests exercise your packages using concurrency
# Example: concurrent non-blocking cache
- This is the problem of memorizing a function, that is, caching the result of a function so that it need be computed only once
- It's possible to build many concurrent structures using either of the 2 approaches - shared variables and locks, or communicating sequential process - without excessive complexity
    - It's not always obvious which approach is preferable in a given situation, but it's worth knowing how they correspond
    - Sometimes switching from one approach to the other can make the code simpler
# Goroutines and threads