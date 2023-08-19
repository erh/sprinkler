package main

import (
	"context"
	_ "embed"
	"flag"

	"github.com/edaniels/golog"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/robot/client"
	"go.viam.com/rdk/utils"
	"go.viam.com/utils/rpc"

	"github.com/erh/sprinkler"
)

func main() {
	err := realMain()
	if err != nil {
		panic(err)
	}

}

func realMain() error {
	ctx := context.Background()

	var host, secret string
	flag.StringVar(&host, "host", "", "robot host")
	flag.StringVar(&secret, "secret", "", "robot secret")
	flag.Parse()

	logger := golog.NewDevelopmentLogger("client")

	robot, err := client.New(
		context.Background(),
		host,
		logger,
		client.WithDialOptions(rpc.WithCredentials(rpc.Credentials{
			Type:    utils.CredentialsTypeRobotLocationSecret,
			Payload: secret,
		})),
	)
	if err != nil {
		return err
	}

	mySprinkler, err := sensor.FromRobot(robot, "sprinkler")
	if err != nil {
		return err
	}

	return sprinkler.RunServer(ctx, logger, ":8888", mySprinkler)
}
