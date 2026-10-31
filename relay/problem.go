package relay

import (
	"fmt"
	"strings"
)

// ProblemKind identifies why hosting can't be reached from the internet.
type ProblemKind int

const (
	// ProblemNoUPnP: the router didn't answer automatic setup. Fixable by
	// forwarding the ports by hand.
	ProblemNoUPnP ProblemKind = iota + 1
	// ProblemRefused: the router answered but wouldn't open the ports.
	ProblemRefused
	// ProblemConflict: another device already has these ports forwarded.
	ProblemConflict
	// ProblemCGNAT: the ISP shares one public address between customers.
	// Port forwarding can't fix this.
	ProblemCGNAT
	// ProblemDoubleNAT: the router sits behind another router or modem.
	ProblemDoubleNAT
	// ProblemAddressMismatch: the router's outside address isn't the one the
	// internet sees, usually because of a VPN or ISP-level NAT.
	ProblemAddressMismatch
	// ProblemPortInUse: another program on this PC is using the ports.
	ProblemPortInUse
)

// Problem is a plain-English explanation of why friends outside this
// network may not be able to connect, with steps the user can follow.
type Problem struct {
	Kind    ProblemKind
	Title   string
	Summary string
	Steps   []string // numbered steps; empty when there's nothing to do here
	Tip     string
	// CanForward is true when forwarding ports by hand would fix it, so the
	// UI can offer a link to router-specific guides.
	CanForward bool
	Details    string // technical error text for the "details" section
}

// Text renders the problem as plain text for the "Copy these steps" button.
func (p *Problem) Text() string {
	var b strings.Builder
	b.WriteString(p.Title + "\n\n" + p.Summary + "\n")
	for i, s := range p.Steps {
		fmt.Fprintf(&b, "\n%d. %s", i+1, s)
	}
	if len(p.Steps) > 0 {
		b.WriteString("\n")
	}
	if p.Tip != "" {
		b.WriteString("\nTip: " + p.Tip + "\n")
	}
	return b.String()
}

// netFacts is what we know about the network when building a Problem.
type netFacts struct {
	localIP  string // this PC on the home network
	routerIP string // default gateway, "" if unknown
	ports    []int
}

func (f netFacts) portRange() string {
	if len(f.ports) == 0 {
		return ""
	}
	if len(f.ports) == 1 {
		return fmt.Sprint(f.ports[0])
	}
	return fmt.Sprintf("%d–%d", f.ports[0], f.ports[len(f.ports)-1])
}

func (f netFacts) routerPage() string {
	if f.routerIP == "" {
		return "your router's settings page. Its address is usually printed on a sticker on the router, often http://192.168.0.1 or http://192.168.1.1"
	}
	return "http://" + f.routerIP + " in your browser"
}

// forwardSteps are the manual port-forwarding instructions, filled in with
// this PC's details.
func (f netFacts) forwardSteps() []string {
	return []string{
		"Open " + f.routerPage() + " and sign in to your router. The login is often printed on a sticker on the router.",
		"Find the Port Forwarding section. It may be called Virtual Server, NAT, Port Mapping, or Applications & Gaming, and is often under Advanced.",
		fmt.Sprintf("Add a new rule. Device / IP address: %s (this PC). Ports: %s (both the outside and inside ports). Protocol: TCP.", f.localIP, f.portRange()),
		"Save the rule, then click Try again here.",
		"If Windows asks whether to allow Troopers Transfer through the firewall, click Allow.",
	}
}

func problemNoUPnP(f netFacts, details string) *Problem {
	return &Problem{
		Kind:  ProblemNoUPnP,
		Title: "Couldn't open the ports automatically",
		Summary: "Your router didn't respond to the automatic setup (UPnP). It may be turned off, or your router may not support it. " +
			"You can open the ports yourself instead. It only takes a few minutes and you only need to do it once.",
		Steps:      f.forwardSteps(),
		Tip:        "Some routers have a UPnP setting you can switch on instead. Look for it under Advanced or NAT settings, then click Try again.",
		CanForward: true,
		Details:    details,
	}
}

func problemRefused(f netFacts, port int, details string) *Problem {
	return &Problem{
		Kind:  ProblemRefused,
		Title: "Your router wouldn't open the ports",
		Summary: fmt.Sprintf("Your router answered the automatic setup but refused to open port %d. "+
			"Some routers only allow automatic setup for certain devices or ports. You can open the ports yourself instead.", port),
		Steps:      f.forwardSteps(),
		CanForward: true,
		Details:    details,
	}
}

func problemConflict(f netFacts, port int, details string) *Problem {
	return &Problem{
		Kind:  ProblemConflict,
		Title: "Another device is already using these ports",
		Summary: fmt.Sprintf("Your router already sends port %d to a different device on your network, so it can't send it to this PC too. "+
			"Pick a different starting port in the Host tab and try again, or remove the old rule in your router.", port),
		Steps: []string{
			"Change Starting port in the Host tab to something else, for example 9109, and click Start hosting again.",
			"Or open " + f.routerPage() + ", find Port Forwarding, and delete the old rule for these ports.",
		},
		CanForward: true,
		Details:    details,
	}
}

func problemCGNAT(details string) *Problem {
	return &Problem{
		Kind:  ProblemCGNAT,
		Title: "Hosting won't work on this connection",
		Summary: "Your internet provider shares one public address between several customers (carrier-grade NAT), so people outside your home can't connect to your PC, " +
			"even with port forwarding. This is common on mobile, satellite and some cable or fibre plans.",
		Steps: []string{
			"Ask your friend to host instead, if their connection allows it.",
			"Or keep using the public relays (Connect tab → Use public relays). They work on any connection.",
			"Some providers will give you your own public IP address if you ask, sometimes for a small fee.",
		},
		Tip:     "Friends on the same Wi-Fi or home network as you can still connect using the local connection code.",
		Details: details,
	}
}

func problemDoubleNAT(f netFacts, routerExternal, details string) *Problem {
	return &Problem{
		Kind:  ProblemDoubleNAT,
		Title: "Your router is behind another router",
		Summary: fmt.Sprintf("Your router's outside address is %s, which is a home-network address. "+
			"That means it's plugged into another router or an ISP modem that also does routing, so the ports would have to be opened on both. ", routerExternal) +
			"The simplest fix is usually to put the ISP modem in bridge mode, or to forward the ports on it to your router.",
		Steps: []string{
			"Open the settings page of the device your router is plugged into (often your ISP's modem). Its address is often " + routerExternalGateway(routerExternal) + ".",
			fmt.Sprintf("Forward TCP ports %s to your router's outside address, %s.", f.portRange(), routerExternal),
			"Then click Try again here.",
		},
		Tip:        "If this is too fiddly, the public relays (Connect tab → Use public relays) work on any connection.",
		CanForward: true,
		Details:    details,
	}
}

// routerExternalGateway guesses the upstream device's address: the .1 of the
// router's outside network.
func routerExternalGateway(ip string) string {
	i := strings.LastIndex(ip, ".")
	if i < 0 {
		return "printed on the device"
	}
	return "http://" + ip[:i] + ".1"
}

func problemMismatch(routerExternal, public, details string) *Problem {
	return &Problem{
		Kind:  ProblemAddressMismatch,
		Title: "Friends may not be able to reach you",
		Summary: fmt.Sprintf("Your router says its outside address is %s, but the internet sees you as %s. "+
			"This usually means a VPN is turned on, or your provider is sharing addresses between customers.", routerExternal, public),
		Steps: []string{
			"If you're using a VPN on this PC, turn it off and click Try again.",
			"If you aren't, your provider is probably using carrier-grade NAT, and hosting won't work from outside your home. The public relays (Connect tab → Use public relays) still work.",
		},
		Tip:     "Friends on the same Wi-Fi or home network as you can still connect using the local connection code.",
		Details: details,
	}
}

func problemPortInUse(port int, details string) *Problem {
	return &Problem{
		Kind:  ProblemPortInUse,
		Title: "Another program is using the relay ports",
		Summary: fmt.Sprintf("Port %d is already in use on this PC, maybe by another copy of Troopers Transfer or by croc. "+
			"Close the other program, or choose a different starting port.", port),
		Steps: []string{
			"Close any other copy of Troopers Transfer or croc that is running.",
			"Or change Starting port in the Host tab, for example to 9109, and click Start hosting again.",
		},
		Details: details,
	}
}
