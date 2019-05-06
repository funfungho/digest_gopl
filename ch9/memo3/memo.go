package memo

import "sync"

// a memo caches the results of calling a Func
type Memo struct {
	f     Func
	mu    sync.Mutex
	cache map[string]result
}

// Func is the type of the function to memorize
type Func func(key string) (interface{}, error)

// []byte; err
type result struct {
	value interface{}
	err   error
}

func New(f Func) *Memo {
	return &Memo{f: f, cache: make(map[string]result)}
}

// `key` is url; `memo.f` is `httpGetBody`
func (memo *Memo) Get(key string) (interface{}, error) {
	// some URLs may be fetched twice; happens when 2 or more goroutines call `GET` for the same URL at about the same time;
	// Both consult the cache, find no value, and then call the slow function `f`
	memo.mu.Lock()
	res, ok := memo.cache[key]
	memo.mu.Unlock()
	if !ok {
		res.value, res.err = memo.f(key)
		memo.mu.Lock()
		memo.cache[key] = res
		memo.mu.Unlock()
	}
	memo.mu.Unlock()
	return res.value, res.err
}