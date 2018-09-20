// Project page; https://github.com/Nordix/mconnect/
// LICENSE; MIT. See the "LICENSE" file in the Project page.
// Copyright (c) 2018, Nordix Foundation

package main

import (
	"os"
	"flag"
	"log"
	"net"
	"fmt"
	"time"
	"math/rand"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"context"
)

const helptext = `
Mconnect make many connects towards an address. The address is
supposed to be a virtual ip-address (vip) that is load-balanced to
many servers. Statistics per server-hostname is printed on exit. The
purpose is testing of connectivity and load balancing.

The "maxconcurrent" should in general be lower than the listen backlog
of the server. The go way of defining the backlog is to take the value
from the;

  /proc/sys/net/core/somaxconn

file which is the max value according to listen(2) (default 128).

Options;
`

type config struct {
	isServer *bool
	addr *string
	src *string
	nconn *int
	keep *bool
	srcmax *int
	seed *int
	maxconcurrent *int
	limiter chan int
	wg sync.WaitGroup
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), helptext)
		flag.PrintDefaults()
	}

	var cmd config
	cmd.isServer = flag.Bool("server", false, "Act as server")
	cmd.addr = flag.String("address", "[::1]:5001", "Server address")
	cmd.src = flag.String("src", "", "Base source address use")
	cmd.srcmax = flag.Int("srcmax", 100, "Number of connect sources")
	cmd.nconn = flag.Int("nconn", 1, "Number of connections")
	cmd.keep = flag.Bool("keep", false, "Keep connections open")
	cmd.seed = flag.Int("seed", 0, "Rnd seed. 0 = init from time")
	cmd.maxconcurrent = flag.Int("maxconcurrent", 64, "Max concurrent connects")

	flag.Parse()
	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(0)
	}

	if *cmd.isServer {
		os.Exit(cmd.server())
	} else {
		os.Exit(cmd.client())
	}
}

func (c *config) client() int {

	if *c.seed != 0 {
		rand.Seed(int64(*c.seed))
	} else {
		rand.Seed(time.Now().UnixNano())
	}

	c.limiter = make(chan int, *c.maxconcurrent)
	go stats_worker(&c.wg)

	// Connects have a default timeout of 2min. So if all connects
	// times out the execution time would be horrible for say 10000
	// connect attempts. So set a dead-line. We assume that 1000
	// connects/sec is supported and we allow a margin of 2 sec.
	var timeout time.Duration
	if *c.keep {
		timeout = time.Hour
	} else {
		timeout = time.Duration(*c.nconn * int(time.Second) / 1000)
		timeout += 2*time.Second
	}
	log.Println("Using timeout;", timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	c.wg.Add(*c.nconn)
	for i := 0; i < *c.nconn; i++ {
		go c.connect(ctx)
	}
	c.wg.Wait()

	c.wg.Add(1)
	hostch <- "QUIT!"
	c.wg.Wait()

	fmt.Println("Failed connects;", failedConnects)
	fmt.Println("Failed reads;", failedReads)
	for h,i := range hostmap {
		fmt.Println(h,i)
	}

	if failedConnects > 0 || failedReads > 0 {
		return 1
	}
	return 0
}

type Server struct {
        hostname string
}

func (c *config) server() int {

	// About listen backlog; /proc/sys/net/core/somaxconn and man listen(2)
	l, err := net.Listen("tcp", *c.addr)
	if err != nil {
		log.Fatal("listen", err)
		return -1
	}
	log.Println("Listen on address; ", *c.addr)
	obj := new(Server)
	if obj.hostname, err = os.Hostname(); err != nil {
		log.Fatal("os.Hostname", err)
		return -1				
	}

	for {
		if conn, err := l.Accept(); err != nil {
			log.Fatal(err)
		} else {
			go obj.handleRequest(conn)
		}
	}

	return 0
}

// Handles incoming requests.
func (obj *Server) handleRequest(conn net.Conn) {
	defer conn.Close()
	conn.Write([]byte(obj.hostname))
	buf := make([]byte, 1024)
	for {
		if _, err := conn.Read(buf); err != nil {
			return
		}
		conn.Write([]byte(obj.hostname))
	}
}



func (c *config) Help() string {
	return helptext
}

func rndAddress(base string, cnt int) (adr net.Addr, err error) {

	var sadr string
	if strings.ContainsAny(base, ":") {
		// ipv6
		if cnt >= 60000 {
			err = errors.New("Address range too large")
			return
		}
		sadr = fmt.Sprintf("[%s:%x]:0", base, rand.Intn(cnt) + 1)
	} else {
		// ipv4
		if cnt > 254 {
			err = errors.New("Address range too large")
			return
		}
		sadr = fmt.Sprintf("%s.%d:0", base, rand.Intn(cnt) + 1)
	}
	adr, err = net.ResolveTCPAddr("tcp", sadr)
	log.Println("Using source address:", sadr)
	return
}

func (c *config) connect(ctx context.Context) {
	defer c.wg.Done()
	var d net.Dialer
	if *c.src != "" {
		if saddr, err := rndAddress(*c.src, *c.srcmax); err != nil {
			log.Fatal(err)
			return
		} else {
			d = net.Dialer{LocalAddr: saddr}
		}
	}
	c.limiter <- 0
	conn, err := d.DialContext(ctx, "tcp", *c.addr)
	<- c.limiter
	if err != nil {
		//log.Println("Connect", err)
		atomic.AddUint64(&failedConnects, 1)
		return
	}
	defer conn.Close()

	// Transfer the context deadline to the connection
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	buf := make([]byte, 4096)
	for ok := true; ok; ok = *c.keep {
		if len, err := conn.Read(buf); err != nil {
			atomic.AddUint64(&failedReads, 1)
			//log.Print("Read", err)
			return
		} else {
			host := string(buf[:len])
			hostch <- host
		}
	}
}


// ----------------------------------------------------------------------
// Stats

var hostmap map[string]int = make(map[string]int)
var hostch chan string = make(chan string, 100)
var failedConnects uint64 = 0
var failedReads uint64 = 0

func stats_worker(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		h := <- hostch
		if h == "QUIT!" {
			return
		}
		if val, ok := hostmap[h]; ok {
			hostmap[h] = (val+1)
		} else {
			hostmap[h] = 1
		}
	}
}
