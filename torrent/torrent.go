package torrent

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"

	bencode "github.com/jackpal/bencode-go"
)

const Bitfield = 5
const Interested = 2
const Unchoke = 1
const Request = 6

type MetaInfo struct {
	Name        string `bencode:"name"`
	Pieces      string `bencode:"pieces"`
	Length      int    `bencode:"length"`
	PieceLength int64  `bencode:"piece length"`
}

type TorrentFile struct {
	Announce string   `bencode:"announce"`
	Info     MetaInfo `bencode:"info"`
}

type Response struct {
	Peers string `bencode:"peers"`
}

type TorrentClient struct {
	File     TorrentFile
	Peers    []string
	InfoHash [20]byte
	PeerID   [20]byte
}

func NewTorrentClient(torrentFilePath string) (*TorrentClient, error) {
	file, err := os.Open(torrentFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open torrent file: %v", err)
	}
	defer file.Close()

	var torrentFile TorrentFile
	if err := bencode.Unmarshal(file, &torrentFile); err != nil {
		return nil, fmt.Errorf("failed to decode torrent file: %v", err)
	}

	infoHash := sha1.Sum(encodeInfo(torrentFile.Info))

	peerID := generatePeerID()

	return &TorrentClient{
		File:     torrentFile,
		InfoHash: infoHash,
		PeerID:   peerID,
	}, nil
}

func encodeInfo(info MetaInfo) []byte {
	var buffer bytes.Buffer
	bencode.Marshal(&buffer, info)
	return buffer.Bytes()
}

func generatePeerID() [20]byte {
	var peerID [20]byte
	copy(peerID[:], "00112233445566778899")
	return peerID
}

func (client *TorrentClient) ConnectTracker() error {
	params := url.Values{}
	params.Add("info_hash", string(client.InfoHash[:]))
	params.Add("peer_id", string(client.PeerID[:]))
	params.Add("port", "6881")
	params.Add("uploaded", "0")
	params.Add("downloaded", "0")
	params.Add("left", strconv.Itoa(client.File.Info.Length))
	params.Add("compact", "1")

	trackerURL := fmt.Sprintf("%s?%s", client.File.Announce, params.Encode())
	resp, err := http.Get(trackerURL)
	if err != nil {
		return fmt.Errorf("failed to get peers data: %v", err)
	}
	defer resp.Body.Close()

	var trackerResponse Response
	if err := bencode.Unmarshal(resp.Body, &trackerResponse); err != nil {
		return fmt.Errorf("failed to decode tracker response: %v", err)
	}

	client.Peers = parsePeers([]byte(trackerResponse.Peers))
	return nil
}

func parsePeers(peersBytes []byte) []string {
	var peers []string
	for i := 0; i < len(peersBytes); i += 6 {
		port := binary.BigEndian.Uint16(peersBytes[i+4 : i+6])
		peers = append(peers, fmt.Sprintf("%d.%d.%d.%d:%d", peersBytes[i], peersBytes[i+1], peersBytes[i+2], peersBytes[i+3], port))
	}
	return peers
}

func (client *TorrentClient) Handshake() (net.Conn, error) {
	peerAddr := client.Peers[0]

	conn, err := net.Dial("tcp", peerAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to peer: %v", err)
	}

	var msg []byte
	msg = append(msg, byte(19))
	msg = append(msg, []byte("BitTorrent protocol")...)
	msg = append(msg, make([]byte, 8)...)
	msg = append(msg, client.InfoHash[:]...)
	msg = append(msg, client.PeerID[:]...)

	if _, err := conn.Write(msg); err != nil {
		return nil, fmt.Errorf("failed to send handshake: %v", err)
	}

	buf := make([]byte, 68)
	if _, err := conn.Read(buf); err != nil {
		return nil, fmt.Errorf("failed to read handshake: %v", err)
	}

	peerID := buf[48:]
	fmt.Printf("Connected to peer: %s, Peer ID: %x\n", peerAddr, peerID)
	return conn, nil
}

func (client *TorrentClient) waitForMessage(conn net.Conn, messageId int) error {
	lengthBuf := make([]byte, 4)

	_, err := conn.Read(lengthBuf)

	if err != nil {
		return fmt.Errorf("failed to read from a peer: %v", err)
	}

	prefixLength := binary.BigEndian.Uint32(lengthBuf)

	message := make([]byte, prefixLength)

	_, err = conn.Read(message)

	if err != nil {
		return fmt.Errorf("failed to read from a peer: %v", err)
	}

	if message[0] != byte(messageId) {
		return fmt.Errorf("wanted to recieve %d message id, but got %d", messageId, message[0])
	}

	return nil
}

func (client *TorrentClient) interested(conn net.Conn) error {
	_, err := conn.Write([]byte{0, 0, 0, 1, Interested})

	if err != nil {
		return fmt.Errorf("failed to write to a peer: %v", err)
	}

	return nil
}

func (client *TorrentClient) Download(outputFileName string) error {
	err := client.ConnectTracker()

	if err != nil {
		return fmt.Errorf("failed to connect to a tracker: %v", err)
	}

	conn, err := client.Handshake()

	if err != nil {
		return fmt.Errorf("failed to do a handshake: %v", err)
	}

	defer conn.Close()

	client.waitForMessage(conn, Bitfield)

	client.interested(conn)

	client.waitForMessage(conn, Unchoke)

	pieceSize := client.File.Info.PieceLength

	pieceCount := int(math.Ceil(float64(client.File.Info.Length) / float64(pieceSize)))

	blockSize := 16 * 1024

	var data []byte

	for i := 0; i < pieceCount; i++ {
		if i == pieceCount-1 {
			pieceSize = int64(client.File.Info.Length) % client.File.Info.PieceLength
		}

		blockCount := int(math.Ceil(float64(pieceSize) / float64(blockSize)))

		piece, err := client.requestPiece(conn, i, pieceSize, blockSize, blockCount)

		if err != nil {
			return fmt.Errorf("failed to download a piece: %v", err)
		}

		data = append(data, piece...)
	}

	if err := os.WriteFile(outputFileName, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %v", err)
	}

	return nil
}

func (client *TorrentClient) requestPiece(conn net.Conn, pieceIndex int, pieceSize int64, blockSize int, blockCount int) ([]byte, error) {
	var data []byte

	for i := 0; i < blockCount; i++ {
		blockLength := blockSize
		if i == blockCount-1 {
			blockLength = int(pieceSize) - (blockCount-1)*blockSize
		}

		piece := struct {
			LengthPrefix uint32
			ID           uint8
			Index        uint32
			Begin        uint32
			Length       uint32
		}{
			LengthPrefix: 13,
			ID:           Request,
			Index:        uint32(pieceIndex),
			Begin:        uint32(i * blockSize),
			Length:       uint32(blockLength),
		}

		var buf bytes.Buffer
		if err := binary.Write(&buf, binary.BigEndian, piece); err != nil {
			return nil, fmt.Errorf("failed to serialize piece request: %v", err)
		}

		if _, err := conn.Write(buf.Bytes()); err != nil {
			return nil, fmt.Errorf("failed to send piece request: %v", err)
		}

		lengthBuf := make([]byte, 4)
		if _, err := conn.Read(lengthBuf); err != nil {
			return nil, fmt.Errorf("failed to read from peer: %v", err)
		}

		prefixLength := binary.BigEndian.Uint32(lengthBuf)
		result := make([]byte, prefixLength)
		if _, err := io.ReadFull(conn, result); err != nil {
			return nil, fmt.Errorf("failed to read from peer: %v", err)
		}

		data = append(data, result[9:]...)
	}

	return data, nil
}
