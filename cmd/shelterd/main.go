package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"shelter-drill-gate/internal/store"
	"shelter-drill-gate/internal/web"
	"shelter-drill-gate/internal/workflow"
)

func main() {
	var addressFlag string
	var databasePath string
	var selfCheck bool
	flag.StringVar(&addressFlag, "addr", "", "回环监听地址")
	flag.StringVar(&databasePath, "db", "shelter-drill-gate.db", "SQLite 数据库路径")
	flag.BoolVar(&selfCheck, "self-check", false, "运行真实 HTTP 业务自检后退出")
	flag.Parse()
	address, err := resolveAddress(addressFlag)
	if err != nil {
		log.Fatal(err)
	}
	if selfCheck {
		if err := runSelfCheck(address); err != nil {
			log.Fatalf("自检失败: %v", err)
		}
		fmt.Println("自检通过：整改复测、独立批准与档案摘要验证完成")
		return
	}
	if err := run(address, databasePath); err != nil {
		log.Fatal(err)
	}
}

func run(address, databasePath string) error {
	repository, err := store.Open(databasePath)
	if err != nil {
		return err
	}
	defer repository.Close()
	service := workflow.New(repository)
	handler := web.New(service).Handler()
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	log.Printf("避难演练认证台已监听 http://%s", address)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case signal := <-signals:
		log.Printf("收到 %s，正在关闭", signal)
	case serveErr := <-serverErrors:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("优雅关闭: %w", err)
	}
	return nil
}
