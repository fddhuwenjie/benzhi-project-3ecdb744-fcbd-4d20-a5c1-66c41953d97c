package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const defaultAddress = "127.0.0.1:19081"

func resolveAddress(flagValue string) (string, error) {
	address := strings.TrimSpace(flagValue)
	if address == "" {
		if portValue := strings.TrimSpace(os.Getenv("PORT")); portValue != "" {
			port, err := strconv.Atoi(portValue)
			if err != nil || port < 1 || port > 65535 {
				return "", fmt.Errorf("PORT 必须是 1 到 65535 的端口号")
			}
			address = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		} else {
			address = defaultAddress
		}
	}
	host, portValue, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("-addr 必须使用 host:port 格式: %w", err)
	}
	port, err := strconv.Atoi(portValue)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("监听端口必须是 1 到 65535")
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return "", fmt.Errorf("监听地址必须是回环地址，拒绝 %q", host)
	}
	return address, nil
}
