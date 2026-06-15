package runtime

import (
	"fmt"
	"net"
	"strconv"
)

func AllocatePort(start int) (int, error) {
	if start <= 0 {
		start = 21435
	}
	for port := start; port <= 65535; port++ {
		addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			continue
		}
		_ = listener.Close()
		return port, nil
	}
	return 0, fmt.Errorf("no free internal port found starting at %d", start)
}
