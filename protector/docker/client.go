package docker

import (
	"context"
	"errors"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/client"
)

var cli *Client

type Client struct {
	client *client.Client
}

//GetClient returns the docker client
func GetClient(endpoint string) (*Client, error) {
	var oldClient *client.Client
	if cli != nil {
		oldClient = cli.client
	}
	client, err := checkAndCreateClient(endpoint, oldClient)
	if err != nil {
		return nil, err
	}
	cli = &Client{client: client}
	return cli, nil
}

//checkAndCreateClient
func checkAndCreateClient(endpoint string, cli *client.Client) (*client.Client, error) {
	if cli == nil {
		var err error
		if endpoint == "" {
			cli, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.24"))
		} else {
			cli, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.24"), client.WithHost(endpoint))
		}
		if err != nil {
			return nil, err
		}
	}
	return ping(cli)
}

// ping
func ping(cli *client.Client) (*client.Client, error) {
	if cli == nil {
		return nil, errors.New("client is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	p, err := cli.Ping(ctx)
	if err == nil {
		return cli, nil
	}
	if p.APIVersion == "" {
		return nil, err
	}
	// if server version is lower than the client version, downgrade
	if versions.LessThan(p.APIVersion, cli.ClientVersion()) {
		client.WithVersion(p.APIVersion)(cli)
		_, err = cli.Ping(ctx)
		if err == nil {
			return cli, nil
		}
		return nil, err
	}
	return nil, err
}

func (c *Client) GetContainerJSONById(containerId string) (types.ContainerJSON, error) {
	return c.client.ContainerInspect(context.Background(), containerId)
}
