package main

import (
	//"errors"

	"flag"
	"fmt"
	"net/rpc"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"

	//	"fmt"

	"net"
)

// GOL Logic as in Parallel Implementation

func countAliveCells(p stubs.Params, world [][]byte) int {
	count := 0
	for _, row := range world {
		for _, cellValue := range row {
			if cellValue == 255 {
				count++
			}
		}
	}

	return count
}

func callWorker(y1, y2 int, p stubs.Params, world [][]uint8, ch chan<- []util.Cell, client *rpc.Client) {
	request := stubs.Request{CurrentState: world, Params: stubs.Params(p), Y1: y1, Y2: y2}
	response := new(stubs.Response)
	client.Call(stubs.ProcessSlice, request, response)
	ch <- response.FlippedCells
}

func (g *Gol) calculateNewState(p stubs.Params) []util.Cell {
	// Make new 2D array for the next frame
	var flippedCells []util.Cell

	channels := make([]chan []util.Cell, p.Threads)
	for v := range channels {
		channels[v] = make(chan []util.Cell)
	}

	// Values for dividing up world between n threads
	sliceSize := p.ImageHeight / p.Threads
	remainder := p.ImageHeight % p.Threads

	for i, channel := range channels {
		i += 1
		// Calculate y bounds for thread
		y1 := (i - 1) * sliceSize
		y2 := (i * sliceSize) - 1
		if i == p.Threads {
			y2 += remainder
		}
		// Start worker on its slice
		go callWorker(y1, y2, p, g.state, channel, g.clients[i-1])

	}

	for _, channel := range channels {
		newSection := <-channel
		flippedCells = append(flippedCells, newSection...)
	}

	g.lock.Lock()
	for _, cell := range flippedCells {
		g.state[cell.Y][cell.X] = 255 - g.state[cell.Y][cell.X]
	}
	g.lock.Unlock()

	return flippedCells

}

// distributor divides the work between workers and interacts with other goroutines.
// func engine(p stubs.Params, state [][]uint8) [][]uint8 {

// 	// Channel to receive new state output from workers

// }

// RPC Requests

type Gol struct {
	state   [][]uint8
	turn    int
	lock    sync.Mutex
	clients []*rpc.Client
	pause   bool
	quit    bool
	signal  chan string
	wg      sync.WaitGroup
}

func (g *Gol) ProcessTurn(req stubs.Request, res *stubs.Response) (err error) {
	g.wg.Add(1)
	defer g.wg.Done()

	req.Params.Threads = 2

	if req.CurrentState != nil {
		g.state = req.CurrentState
		g.turn = 0
	}

	cellsFlipped := g.calculateNewState(req.Params)
	res.FlippedCells = cellsFlipped

	res.CurrentTurn = g.turn

	if g.turn == req.Params.Turns-1 {
		res.State = g.state
	}

	g.turn += 1

	return
}

// calculate new state
// func (g *Gol) ProcessTurns(req stubs.Request, res *stubs.Response) (err error) {
// 	g.wg.Add(1)
// 	defer g.wg.Done()

// 	req.Params.Threads = 2

// 	g.quit = false

// 	// If we're not paused because of a client quit, start from new state.
// 	// Otherwise, it will just resume processing on the already existing state
// 	if g.pause == false {
// 		g.state = req.CurrentState
// 		g.turn = 0
// 	}

// 	// Maybe find proper way to say g.turn = g.turn?
// 	for g.turn = g.turn; g.turn < req.Params.Turns && g.quit == false; g.turn++ {
// 		newFrame := calculateNewState(req.Params, g)
// 		g.lock.Lock()
// 		g.state = newFrame
// 		g.lock.Unlock()
// 	}

// 	res.State = g.state
// 	res.CurrentTurn = g.turn

// 	return
// }

// alive cells count called by the distributor
func (g *Gol) AliveCellsCount(req stubs.Request, res *stubs.Response) (err error) {
	g.wg.Add(1)
	defer g.wg.Done()

	g.lock.Lock()
	count := countAliveCells(req.Params, g.state)
	g.lock.Unlock()
	res.CurrentTurn = g.turn
	res.CellCount = count

	return
}

func (g *Gol) Screenshot(req stubs.Request, res *stubs.Response) (err error) {
	g.wg.Add(1)
	defer g.wg.Done()

	g.lock.Lock()
	res.State = g.state
	g.lock.Unlock()
	res.CurrentTurn = g.turn

	return
}

func (g *Gol) PauseBroker(req stubs.Request, res *stubs.Response) (err error) {
	g.wg.Add(1)
	defer g.wg.Done()

	if g.pause == false {
		g.lock.Lock()
		g.pause = true
	} else {
		g.lock.Unlock()
		g.pause = false
	}

	res.CurrentTurn = g.turn
	res.Paused = g.pause

	return
}

func (g *Gol) ClientQuit(req stubs.Request, res *stubs.Response) (err error) {
	g.wg.Add(1)
	defer g.wg.Done()

	g.lock.Lock()
	res.State = g.state
	g.quit = true
	g.lock.Unlock()
	res.CurrentTurn = g.turn
	g.pause = true

	return
}

func (g *Gol) KillBroker(req stubs.Request, res *stubs.Response) (err error) {
	g.wg.Add(1)
	defer g.wg.Done()

	for _, client := range g.clients {
		req := new(stubs.Request)
		response := new(stubs.Response)
		client.Call(stubs.KillWorker, req, response)
	}

	g.lock.Lock()
	res.State = g.state
	res.CurrentTurn = g.turn
	g.quit = true
	g.lock.Unlock()

	g.signal <- "KILL"

	return
}

// Ensure clients have closed connections (and therefore have received responses) before closing server
func (g *Gol) serveConn(conn net.Conn) {
	g.wg.Add(1)
	defer g.wg.Done()
	rpc.ServeConn(conn)
}

func (g *Gol) startAccepting(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			// If error is caused by us having intentionally closed the server, return
			if g.quit {
				return
			} else {
				fmt.Println("Accept error:", err)
			}
		} else {
			go g.serveConn(conn)
		}
	}
}

// Server Handling
func main() {
	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()

	instances := []string{"3.89.204.130:8030", "54.237.230.235:8030"}
	connections := []*rpc.Client{}

	for _, instance := range instances {
		client, _ := rpc.Dial("tcp", instance)
		connections = append(connections, client)
	}

	listener, _ := net.Listen("tcp", ":"+*pAddr)
	g := Gol{clients: connections, signal: make(chan string, 1)}
	rpc.Register(&g)
	fmt.Println("Server open on port", *pAddr)
	defer listener.Close()
	go g.startAccepting(listener)
	<-g.signal
	fmt.Println("Server closing...")
	g.wg.Wait()
	for _, client := range g.clients {
		client.Close()
	}
	close(g.signal)
}
