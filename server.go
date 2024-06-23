package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	CONN_PORT = ":3334"
	CONN_TYPE = "tcp"
)

type Client struct {
	conn     net.Conn
	username string
	room     string
}

type BannedUser struct {
	Address string
}

var (
	clients     = make(map[net.Conn]*Client)
	rooms       = make(map[string][]*Client)
	broadcast   = make(chan string)
	mutex       = &sync.Mutex{}
	bannedUsers = make(map[string]BannedUser)
)

func handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	client := &Client{conn: conn, username: "Anonymous"}

	mutex.Lock()
	clients[conn] = client
	mutex.Unlock()

	if _, banned := bannedUsers[conn.RemoteAddr().String()]; banned {
		conn.Write([]byte("You are banned from the chat.\n"))
		conn.Close()
		return
	}

	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Client disconnected: %v", conn.RemoteAddr())
			mutex.Lock()
			if client.room != "" {
				rooms[client.room] = removeClient(rooms[client.room], client)
				broadcast <- fmt.Sprintf("[%s] Notice: \"%s\" left the chat room.\n", client.room, client.username)
			}
			delete(clients, conn)
			mutex.Unlock()
			return
		}
		message = strings.TrimSpace(message)
		if strings.HasPrefix(message, "/") {
			handleCommand(message, client)
		} else {
			if client.room == "" {
				conn.Write([]byte("You must join a room first using /join [room_name] or create a room using /create [room_name].\n"))
			} else {
				broadcast <- fmt.Sprintf("[%s] %s - %s: %s\n", client.room, time.Now().Format("3:04PM"), client.username, message)
			}
		}
	}
}

func handleCommand(message string, client *Client) {
	parts := strings.Split(message, " ")
	command := parts[0]

	switch command {
	case "/join":
		if len(parts) < 2 {
			client.conn.Write([]byte("Usage: /join [room_name]\n"))
			return
		}
		roomName := parts[1]
		mutex.Lock()
		if _, exists := rooms[roomName]; !exists {
			client.conn.Write([]byte(fmt.Sprintf("Room %s does not exist. Use /create [room_name] to create a new room.\n", roomName)))
			mutex.Unlock()
			return
		}
		if _, banned := bannedUsers[client.conn.RemoteAddr().String()]; banned {
			client.conn.Write([]byte("You are banned from the chat.\n"))
			mutex.Unlock()
			return
		}
		if client.room != "" {
			rooms[client.room] = removeClient(rooms[client.room], client)
			broadcast <- fmt.Sprintf("[%s] Notice: \"%s\" left the chat room.\n", client.room, client.username)
		}
		client.room = roomName
		rooms[roomName] = append(rooms[roomName], client)
		mutex.Unlock()
		client.conn.Write([]byte(fmt.Sprintf("Joined room %s\n", roomName)))
		broadcast <- fmt.Sprintf("[%s] Notice: \"%s\" joined the chat room.\n", roomName, client.username)

	case "/create":
		if len(parts) < 2 {
			client.conn.Write([]byte("Usage: /create [room_name]\n"))
			return
		}
		roomName := parts[1]
		mutex.Lock()
		if _, exists := rooms[roomName]; exists {
			client.conn.Write([]byte(fmt.Sprintf("Room %s already exists. Use /join [room_name] to join the room.\n", roomName)))
			mutex.Unlock()
			return
		}
		if _, banned := bannedUsers[client.conn.RemoteAddr().String()]; banned {
			client.conn.Write([]byte("You are banned from the chat.\n"))
			mutex.Unlock()
			return
		}
		rooms[roomName] = []*Client{}
		if client.room != "" {
			rooms[client.room] = removeClient(rooms[client.room], client)
			broadcast <- fmt.Sprintf("[%s] Notice: \"%s\" left the chat room.\n", client.room, client.username)
		}
		client.room = roomName
		rooms[roomName] = append(rooms[roomName], client)
		mutex.Unlock()
		client.conn.Write([]byte(fmt.Sprintf("Created and joined room %s\n", roomName)))
		broadcast <- fmt.Sprintf("[%s] Notice: \"%s\" created and joined the chat room.\n", roomName, client.username)

	case "/help":
		helpMessage := "/join [room_name] - Join a room\n" +
			"/create [room_name] - Create a room\n" +
			"/help - Show this help message\n"
		client.conn.Write([]byte(helpMessage))

	default:
		client.conn.Write([]byte("Unknown command. Type /help for a list of commands.\n"))
	}
}

func removeClient(slice []*Client, client *Client) []*Client {
	for i, c := range slice {
		if c == client {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func handleBroadcast() {
	for {
		message := <-broadcast
		parts := strings.SplitN(message, " ", 3)
		room := parts[0][1 : len(parts[0])-1]
		mutex.Lock()
		for _, client := range rooms[room] {
			_, err := client.conn.Write([]byte(message))
			if err != nil {
				log.Printf("Error sending message to client %v: %v", client.conn.RemoteAddr(), err)
				client.conn.Close()
				delete(clients, client.conn)
				rooms[room] = removeClient(rooms[room], client)
			}
		}
		mutex.Unlock()
	}
}

func adminConsole() {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Admin Command > ")
		command, _ := reader.ReadString('\n')
		command = strings.TrimSpace(command)

		switch command {
		case "/clients":
			printClients()
		case "/rooms":
			printRooms()
		case "/stats":
			printStats()
		case "/help":
			printAdminHelp()
		case "/kick":
			fmt.Print("Enter IP address to kick: ")
			ip, _ := reader.ReadString('\n')
			ip = strings.TrimSpace(ip)
			for conn := range clients {
				addr := conn.RemoteAddr().String()
				if addr == ip {
					kickUser(conn)
					fmt.Printf("User %s has been kicked from the chat.\n", ip)
					break
				}
			}
		case "/ban":
			fmt.Print("Enter IP address to ban: ")
			ip, _ := reader.ReadString('\n')
			ip = strings.TrimSpace(ip)
			for conn := range clients {
				addr := conn.RemoteAddr().String()
				if addr == ip {
					banUser(conn)
					fmt.Printf("User %s has been banned from the chat.\n", ip)
					break
				}
			}
		default:
			fmt.Println("Unknown command. Type /help for a list of commands.")
		}
	}
}

func kickUser(conn net.Conn) {
	for _, roomClients := range rooms {
		for i, client := range roomClients {
			if client.conn == conn {
				rooms[client.room] = append(roomClients[:i], roomClients[i+1:]...)
				client.room = ""
				conn.Write([]byte("You have been kicked from the chat.\n"))
				return
			}
		}
	}
}

func banUser(conn net.Conn) {
	connAddr := conn.RemoteAddr().String()
	bannedUsers[connAddr] = BannedUser{
		Address: connAddr,
	}
	kickUser(conn)
}

func printClients() {
	mutex.Lock()
	defer mutex.Unlock()

	if len(clients) == 0 {
		fmt.Println("No clients connected.")
		return
	}

	fmt.Println("Connected clients:")
	for _, client := range clients {
		fmt.Printf("Client: %s, Room: %s\n", client.conn.RemoteAddr(), client.room)
	}
}

func printRooms() {
	mutex.Lock()
	defer mutex.Unlock()

	if len(rooms) == 0 {
		fmt.Println("No active rooms.")
		return
	}

	fmt.Println("Active rooms:")
	for roomName, clients := range rooms {
		fmt.Printf("Room: %s, Members: %d\n", roomName, len(clients))
		for _, client := range clients {
			fmt.Printf(" - %s\n", client.conn.RemoteAddr())
		}
	}
}

func printStats() {
	mutex.Lock()
	defer mutex.Unlock()

	fmt.Printf("Server Stats:\n")
	fmt.Printf("Total clients connected: %d\n", len(clients))
	fmt.Printf("Total rooms: %d\n", len(rooms))
}

func printAdminHelp() {
	fmt.Println("Available commands:")
	fmt.Println("  /clients  - List all connected clients")
	fmt.Println("  /rooms    - List all chat rooms and their members")
	fmt.Println("  /stats  - Show server statistics")
	fmt.Println("  /kick   - Kick a user from the server")
	fmt.Println("  /ban    - Ban a user from the server")
	fmt.Println("  /help   - Show this help message")
}

func main() {
	cert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
	if err != nil {
		log.Fatal(err)
	}

	config := &tls.Config{Certificates: []tls.Certificate{cert}}
	listener, err := tls.Listen(CONN_TYPE, CONN_PORT, config)
	if err != nil {
		log.Println("Error: ", err)
		os.Exit(1)
	}
	defer listener.Close()
	log.Println("Listening on " + CONN_PORT)

	go handleBroadcast()
	go adminConsole()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Error: ", err)
			continue
		}
		log.Printf("Client connected: %v", conn.RemoteAddr())
		go handleConnection(conn)
	}
}
