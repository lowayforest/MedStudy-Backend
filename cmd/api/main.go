package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/common-nighthawk/go-figure"
	"github.com/lowayforest/MedStudy-Backend/internal/config"
	"github.com/lowayforest/MedStudy-Backend/internal/platform/postgres"
)

func main() {

	err := run(".env")

	if err != nil {
		log.Fatal(err)
	}

}

func newRouter() *http.ServeMux {
	router := http.NewServeMux()
	router.HandleFunc("GET /health/live", healthLive)

	return router
}

func run(filename string) error {
	//读取配置文件
	cfg, err := config.LoadFile(filename)
	if err != nil {
		return err
	}

	//设置连接时间
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//初始化连接池
	pool, err := postgres.Open(ctx, cfg.DatabaseURL, cfg.DatabasePassword)
	if err != nil {
		return errors.New("数据库初始化失败，请检查配置和数据库服务")
	}

	//退出函数时关闭连接池
	defer pool.Close()
	fmt.Println("数据库连接成功！")

	//创建路由，启动HTTP服务

	router := newRouter()
	fmt.Println("准备启动后端")
	fmt.Println("访问: http://localhost:11345/health/live")

	// 打印 LOWAYFOREST Logo
	logo := figure.NewFigure("LOWAYFOREST", "slant", true)

	fmt.Println("\033[1;36m")
	logo.Print()
	fmt.Println("\033[0m")

	fmt.Println("🚀 LOWAYFOREST Backend Starting...")

	return http.ListenAndServe("127.0.0.1:11345", router)

}
