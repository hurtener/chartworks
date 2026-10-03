//go:build ignore

package main

import "os"

func main() { _, _ = os.Stdout.Write(make([]byte, 4096)) }
