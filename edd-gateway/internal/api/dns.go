package api

import "net"

// lookupTXT is a package var so tests can stub DNS.
var lookupTXT = net.LookupTXT
