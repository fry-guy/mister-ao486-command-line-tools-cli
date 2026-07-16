package main

import "encoding/base64"

// mbrBootstrapB64 is the exact 446-byte real-mode MBR bootstrap code
// used by every VHD this toolkit creates (originally shipped as the
// standalone file mbr_bootstrap.bin, read from disk by make_mbr.py).
// Embedding it as a base64 string keeps the whole toolkit a single
// binary with zero sidecar files to lose track of.
const mbrBootstrapB64 = "+jHAjtC8AHz7/LgAAI7YjsC+AHy/AAa5AAHzpeohBgAAiBbgBr6+B7kEAIA8gHQLg8YQ4va+8wbpmACJNuEGtEG7qlWKFuAGzRNyR4H7Vap1QfbBAXQ8izbhBmaLRAhmo+sGZscG7wYAAAAAxwbjBhAAxwblBgEAxwbnBgB8xwbpBgAAvuMGihbgBrRCzRNyOOshizbhBop0AYpMAopsA7gAAI7AuwB8tAKwAYoW4AbNE3IVgT7+fVWqdRKLNuEGihbgBuoAfAAAvgcH6wW+FwfrALQOrITAdATNEOv36/4AAAAQAAAAAAAAAAAAAAAAAAAATm8gYWN0aXZlIHBhcnRpdGlvbgBCb290IHJlYWQgZXJyb3IAQmFkIGJvb3Qgc2lnbmF0dXJlAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

func mbrBootstrap() []byte {
	b, err := base64.StdEncoding.DecodeString(mbrBootstrapB64)
	if err != nil {
		panic("corrupt embedded mbr bootstrap: " + err.Error())
	}
	if len(b) != 446 {
		panic("embedded mbr bootstrap is not 446 bytes")
	}
	return b
}
