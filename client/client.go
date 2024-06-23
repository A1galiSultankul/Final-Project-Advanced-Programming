package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strings"
)

const (
	SERVER_HOST = "localhost"
	SERVER_PORT = "3334"
	SERVER_TYPE = "tcp"
)

func main() {
	// Configure TLS settings
	config := &tls.Config{
		InsecureSkipVerify: true, // For testing purposes only, should use proper CA verification in production
	}

	// Connect to server
	connection, err := tls.Dial(SERVER_TYPE, SERVER_HOST+":"+SERVER_PORT, config)
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		return
	}
	defer connection.Close()

	fmt.Println("Connected to chat server")

	// Create a channel to read input from the console
	input := make(chan string)
	go readInput(input)

	// Create a channel to read messages from the server
	messages := make(chan string)
	go readMessages(connection, messages)

	for {
		select {
		case msg := <-input:
			if strings.TrimSpace(msg) == "/quit" {
				fmt.Println("Disconnecting from chat server...")
				return
			}
			_, err := connection.Write([]byte(msg + "\n"))
			if err != nil {
				fmt.Println("Error sending message:", err)
				return
			}
		case msg := <-messages:
			fmt.Println(msg)
		}
	}
}

func readInput(input chan<- string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		input <- scanner.Text()
	}
	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading from console:", err)
	}
}

func readMessages(conn net.Conn, messages chan<- string) {
	reader := bufio.NewReader(conn)
	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading from server:", err)
			close(messages)
			return
		}
		messages <- strings.TrimSpace(message)
	}
}
