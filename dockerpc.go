package dockerpc

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/rpc"
	"net/rpc/jsonrpc"
	"net/url"
	"os"

	docker "github.com/fsouza/go-dockerclient"
)

//
// dockerpc Client data
//
type Client struct {
	ID           string       // internal ID of docker container
	name         string       // name to use
	dockerImage  string       // docker image to use
	stdErrBuf    bytes.Buffer // buffer for storing stderr logs
	endpoint     string
	output       io.Writer
	dockerClient *docker.Client
	rpcClient    *rpc.Client
	clientConn   net.Conn

	DockerHostConfig *docker.HostConfig // host config parameters when starting docker
	DockerConfig     *docker.Config     // config parameters when starting docker
}

// Create a new dockerpc Client client
func NewClient(name string, dockerImage string, endpoint string) *Client {
	ret := &Client{
		name:        name,
		dockerImage: dockerImage,
		endpoint:    endpoint,
	}
	return ret
}

// Close will remove the container, and close any client resources
func (d *Client) Close() error {

	if d.dockerClient != nil {
		opts := docker.RemoveContainerOptions{ID: d.ID, Force: true}
		d.dockerClient.RemoveContainer(opts)
	}

	if d.rpcClient != nil {
		err := d.rpcClient.Close()
		d.rpcClient = nil
		if err != nil {
			return err
		}
	}

	if d.rpcClient != nil {
		err := d.rpcClient.Close()
		d.rpcClient = nil
		if err != nil {
			return err
		}
	}
	return nil
}

// AttachStreamingContainer will attach to a container.
func (d *Client) AttachStreamingContainer(opts docker.AttachToContainerOptions) error {
	uri := "/containers/" + opts.Container + "/attach?" + queryString(opts)
	u, err := url.Parse(d.endpoint + uri)

	if err != nil {
		return err
	}

	var rawConn net.Conn
	if d.dockerClient.TLSConfig != nil {
		rawConn, err = tls.Dial("tcp", u.Host, d.dockerClient.TLSConfig)
	} else {
		rawConn, err = net.Dial("tcp", u.Host)
	}
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	req, err := http.NewRequest("POST", uri, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "plain/text")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "tcp")

	clientconn := httputil.NewClientConn(rawConn, nil)
	resp, err := clientconn.Do(req)

	if err != nil {
		log.Println("Error response from socket", resp)
		return err
	}

	d.clientConn, _ = clientconn.Hijack()

	return nil
}

// Call calls a method on the RPC server running on the client.
func (d *Client) Call(method string, args interface{}, reply interface{}) error {
	d.stdErrBuf.Reset()
	return d.rpcClient.Call(method, args, reply)
}

func (d *Client) StdError() string {
	return string(d.stdErrBuf.Bytes())
}

//
// Start a docker container, and create a connection to /attach to it and send
// and receive RPC commands.
//
func (d *Client) Start() (err error) {

	path := os.Getenv("DOCKER_CERT_PATH")
	if path != "" {
		ca := fmt.Sprintf("%s/ca.pem", path)
		cert := fmt.Sprintf("%s/cert.pem", path)
		key := fmt.Sprintf("%s/key.pem", path)
		d.dockerClient, err = docker.NewTLSClient(d.endpoint, cert, key, ca)
	} else {
		d.dockerClient, err = docker.NewClient(d.endpoint)
	}

	if err != nil {
		return err
	}

	defaultConfig := &docker.Config{
		OpenStdin: true,
		Image:     d.dockerImage,
	}

	defaultHostConfig := &docker.HostConfig{}

	opts := docker.CreateContainerOptions{
		Config:     defaultConfig,
		HostConfig: defaultHostConfig,
	}

	if d.name != "" {
		opts.Name = d.name
	}

	c, err := d.dockerClient.CreateContainer(opts)

	if err != nil {
		return err
	}

	d.ID = c.ID
	err = d.dockerClient.StartContainer(c.ID, defaultHostConfig)

	if err != nil {
		return err
	}

	attachOpts := docker.AttachToContainerOptions{
		Container: d.ID,
		Stdout:    true,
		Stdin:     true,
		Stderr:    true,
		Stream:    true,
	}

	err = d.AttachStreamingContainer(attachOpts)

	if err != nil {
		return err
	}

	pipes := &dockerPipes{
		d.clientConn,
		&d.stdErrBuf,
		0,
		0,
	}

	d.rpcClient = rpc.NewClientWithCodec(jsonrpc.NewClientCodec(pipes))

	return nil
}

const (
	STDIN  = 0
	STDOUT = 1
	STDERR = 2
)

// todo close everything
type dockerPipes struct {
	conn           net.Conn
	stdErrBuf      *bytes.Buffer
	bytesRemaining uint32
	pipeName       byte
}

func (pipe *dockerPipes) Read(b []byte) (int, error) {
	// try to read n bytes from the connection
	// this is the Docker header as described here:
	// https://docs.docker.com/reference/api/docker_remote_api_v1.20/#attach-to-a-container
	if pipe.bytesRemaining == 0 {
		var p []byte = make([]byte, 1024)
		c, err := pipe.conn.Read(p)
		if err != nil {
			return 0, err
		}

		if c != 8 {
			return 0, errors.New("Expected 8 byte header from Docker")
		}
		var size uint32
		sizeReader := bytes.NewReader(p[4:])
		binary.Read(sizeReader, binary.BigEndian, &size)

		pipe.pipeName = p[0]
		pipe.bytesRemaining = size
	}

	pipeName := pipe.pipeName
	c, err := pipe.conn.Read(b)

	if err != nil {
		return 0, err
	}

	// handle if the supplied byte array is smaller than the total size.
	// if we haven't read the full # of bytes, then on the next Read()
	// don't try to re-read the Docker header, simply continue reading the buffer.
	bufSize := uint32(len(b))

	if bufSize < pipe.bytesRemaining {
		pipe.bytesRemaining = pipe.bytesRemaining - bufSize
	} else {
		pipe.bytesRemaining = 0
	}

	switch pipeName {
	case STDIN:
		return 0, nil
	case STDOUT:
		return c, nil
	case STDERR:
		// standard error - write it to buf.
		pipe.stdErrBuf.Write(b[0:c])
		return 0, nil
	}

	return 0, errors.New(fmt.Sprintf("Unsupported pipe: %d ", pipeName))
}

func (pipe *dockerPipes) Write(b []byte) (int, error) {
	return pipe.conn.Write(b)
}

func (pipe *dockerPipes) Close() error {
	return pipe.conn.Close()
}
