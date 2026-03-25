package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

type Resp struct {
	id int64 `json:"id"`
}

func main() {
	var wg sync.WaitGroup
	wg.Add(100)

	client := &http.Client{}

	res := make(map[int]int64)
	var mutex sync.Mutex

	for i := 0; i < 100; i++ {
		go func(i int) {
			defer wg.Done()
			r, err := client.Get("http://127.0.0.1:8000/v1/id/test")
			defer r.Body.Close()
			if err != nil {
				log.Fatalf("goroutine %d failed, err: %v", i, err)
			}
			var id int64
			if err = json.NewDecoder(r.Body).Decode(&id); err != nil {
				log.Fatalf("goroutine %d decode res failed, err: %v", i, err)
			}
			mutex.Lock()
			if res[int(id)] == 0 {
				res[int(id)]++
			} else {
				log.Fatalf("goroutine %d repeat id, id: %v", i, id)
			}
			mutex.Unlock()
		}(i)
	}
	wg.Wait()
	for i, c := range res {
		fmt.Printf("id : %d , count : %d\n", i, c)
	}
	log.Fatal("complete")
}
