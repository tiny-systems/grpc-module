package main

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/goccy/go-json"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tiny-systems/grpc-module/client"
	_ "github.com/tiny-systems/grpc-module/client"
	"github.com/tiny-systems/module/cli"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// RootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "server",
	Short: "tiny-system's gRPC module",
	Run: func(cmd *cobra.Command, args []string) {

		data, err := json.Marshal(&client.Settings{
			Service: client.ServiceName{
				Enum: client.Enum{
					Value:   "servicename",
					Options: []string{"second"},
				},
			},
		})
		if err != nil {
			log.Fatal(err)
		}

		spew.Dump(string(data))
	},
}

func main() {
	// Default level for this example is info, unless debug flag is present
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	viper.AutomaticEnv()
	if viper.GetBool("debug") {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cli.RegisterCommands(rootCmd)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Printf("command execute error: %v\n", err)
	}
}
