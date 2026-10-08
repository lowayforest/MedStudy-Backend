package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {

	router := newRouter()

	fmt.Println("准备启动后端")
	fmt.Println("访问: http://localhost:8080/health/live")

	err := http.ListenAndServe("127.0.0.1:8080", router)

	if err != nil {
		log.Fatalf("HTTP 服务定制: %v", err)
	}
}

func newRouter() *http.ServeMux {
	router := http.NewServeMux()
	router.HandleFunc("GET /health/live", healthLive)

	return router
}
