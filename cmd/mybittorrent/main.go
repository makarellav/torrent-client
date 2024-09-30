package main

import (
	"flag"
	"fmt"

	"github.com/codecrafters-io/bittorrent-starter-go/torrent"
)

func main() {
	torrentFilePath := flag.String("from", "", ".torrent file")
	outputFileName := flag.String("to", "", "output file name")

	flag.Parse()

	client, err := torrent.NewTorrentClient(*torrentFilePath)

	if err != nil {
		fmt.Printf("failed to init a client: %v\n", err)

		return
	}

	err = client.Download(*outputFileName)

	if err != nil {
		fmt.Printf("failed to download a file: %v\n", err)

		return
	}
}
