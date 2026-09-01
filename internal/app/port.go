package app

import (
	"fmt"
	"net"
)

// DynamicPort 表示应用未运行或需要在运行时动态分配端口。
const DynamicPort = 0

// NextPort 获取一个当前可绑定的本地端口。
// 监听器会在返回前关闭，调用方应在获取后尽快使用该端口。
func NextPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("动态分配应用端口失败: %w", err)
	}
	defer listener.Close()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("无法读取动态分配的应用端口")
	}
	return address.Port, nil
}
