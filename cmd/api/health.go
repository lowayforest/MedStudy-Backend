package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func healthLive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	respone := map[string]string{
		"status": "ok",
	}
	err := json.NewEncoder(w).Encode(respone)

	if err != nil {
		log.Printf("写入健康响应检查失败: %v", err)
	}

}
