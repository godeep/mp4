package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jfbus/mp4"
)

func main() {
	start := flag.Int("start", 0, "start time (sec)")
	duration := flag.Int("duration", 0, "duration (sec)")
	flag.Parse()
	in := flag.Arg(0)
	out := flag.Arg(1)
	fd, err := os.Open(in)
	v, err := mp4.Decode(fd)
	if err != nil {
		fmt.Println(err)
	}
	v.Dump()
	if out != "" {
		fd, err = os.Create(out)
		if err != nil {
			fmt.Println(err)
		}
		if *start > 0 {
			v.EncodeFiltered(fd, mp4.Clip(*start, *duration))
		} else {
			//v.EncodeFiltered(fd, mp4.Noop())
			v.EncodeFiltered(fd, mp4.Clip(*start, *duration))
		}
	}
}
