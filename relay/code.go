package relay

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"strings"
)

// DefaultPort is croc's standard relay port. A relay uses it and the next
// four ports.
const DefaultPort = 9009

// PortCount is how many consecutive ports a croc relay listens on: one
// control port plus four transfer ports.
const PortCount = 5

// Ports lists the consecutive TCP ports a relay on base uses.
func Ports(base int) []int {
	ps := make([]int, PortCount)
	for i := range ps {
		ps[i] = base + i
	}
	return ps
}

// ValidPort reports whether base and the following ports are usable.
func ValidPort(base int) bool {
	return base >= 1024 && base+PortCount-1 <= 65535
}

// FormatCode builds the connection code a friend pastes into the Connect
// tab: "host:port/password".
func FormatCode(host string, port int, password string) string {
	return net.JoinHostPort(host, strconv.Itoa(port)) + "/" + password
}

// ParseCode splits a connection code into a relay address and password.
// It also accepts a bare "host:port" (croc's default password is used) and
// a bare host (port 9009).
func ParseCode(code string) (addr, password string, err error) {
	code = strings.TrimSpace(code)
	code = strings.TrimPrefix(code, "tt://")
	if code == "" {
		return "", "", errors.New("enter the connection code your friend gave you")
	}
	if i := strings.LastIndex(code, "/"); i >= 0 {
		code, password = strings.TrimSpace(code[:i]), strings.TrimSpace(code[i+1:])
	}
	host, port, err := net.SplitHostPort(code)
	if err != nil {
		// No port given.
		host, port = code, strconv.Itoa(DefaultPort)
		if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
			return "", "", fmt.Errorf("%q doesn't look like an address", code)
		}
		host = strings.Trim(host, "[]")
	}
	n, err := strconv.Atoi(port)
	if err != nil || !ValidPort(n) {
		return "", "", fmt.Errorf("the port in %q isn't valid", code)
	}
	if host == "" || strings.ContainsAny(host, " /\\") {
		return "", "", fmt.Errorf("%q doesn't look like an address", code)
	}
	return net.JoinHostPort(host, port), password, nil
}

// passwordWords is a short list of easy-to-read words for relay passwords.
var passwordWords = strings.Fields(`
amber anchor apple arrow aspen badge banjo basil beacon birch bison blaze
bolt brave breeze bronze cabin cactus candle canyon cedar cinder clover cobalt
comet coral cosmic crane crisp delta dune eagle ember falcon fern flint forest
fox frost galaxy garnet glacier granite harbor hawk hazel honey iris ivory
jade jasper juniper kayak kettle lagoon lantern lemon lilac lotus lunar maple
marble meadow mesa mint moose nebula nickel north oak olive onyx orbit otter
panda pebble pepper pine planet plum polar prism quartz quill raven reef ridge
river rocket ruby sable saffron sage salmon sierra silver slate solar spruce
summit swift talon tango thunder tiger timber topaz tulip tundra velvet violet
walnut willow winter wolf zephyr`)

// NewPassword returns three random words joined by dashes, like
// "amber-falcon-ridge". Random enough for a relay password and easy to read
// out over voice chat.
func NewPassword() string {
	parts := make([]string, 3)
	max := big.NewInt(int64(len(passwordWords)))
	for i := range parts {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			n = big.NewInt(int64(i))
		}
		parts[i] = passwordWords[n.Int64()]
	}
	return strings.Join(parts, "-")
}
