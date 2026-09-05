// Локальный проброс Postgres/Redis из WSL на свободные порты Windows.
//
// WSL2 часто занимает 127.0.0.1:5433 и :6380 «призрачным» пробросом:
// bind на них невозможен, а соединение при этом получает отказ. Поэтому
// туннель слушает 15433 и 16380, а внутри Ubuntu ходит на настоящие 5433/6380.
package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type hop struct {
	listen string
	dest   string
}

func main() {
	relay, err := relayScript()
	if err != nil {
		log.Fatal(err)
	}

	hops := []hop{
		{listen: "15433", dest: "5433"},
		{listen: "16380", dest: "6380"},
	}

	var wg sync.WaitGroup
	for _, item := range hops {
		wg.Add(1)
		go func(item hop) {
			defer wg.Done()
			if err := listen(item, relay); err != nil {
				log.Printf("порт %s: %v", item.listen, err)
			}
		}(item)
	}
	wg.Wait()
}

func listen(item hop, relay string) error {
	ln, err := net.Listen("tcp4", "127.0.0.1:"+item.listen)
	if err != nil {
		return fmt.Errorf("не удалось слушать 127.0.0.1:%s: %w", item.listen, err)
	}
	log.Printf("слушаем 127.0.0.1:%s → WSL 127.0.0.1:%s", item.listen, item.dest)
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go handle(conn, item.dest, relay)
	}
}

func handle(conn net.Conn, destPort, relay string) {
	defer conn.Close()

	cmd := exec.Command("wsl.exe", "-d", "Ubuntu", "--", "python3", relay, destPort)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Printf("wsl python3: %v", err)
		return
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(stdin, conn)
		_ = stdin.Close()
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(conn, stdout)
		done <- struct{}{}
	}()
	<-done
}

func relayScript() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "scripts", "wsl-tcp-relay.py")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return windowsPathToWSL(candidate), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("не найден scripts/wsl-tcp-relay.py")
}

func windowsPathToWSL(p string) string {
	p = filepath.ToSlash(p)
	if len(p) >= 2 && p[1] == ':' {
		return "/mnt/" + strings.ToLower(string(p[0])) + p[2:]
	}
	return p
}
