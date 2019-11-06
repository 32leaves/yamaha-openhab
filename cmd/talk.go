package cmd

import (
	"fmt"

	"github.com/32leaves/yamaha-openhab/pkg/musiccast"
	"github.com/spf13/cobra"
)

// talkCmd represents the talk command
var talkCmd = &cobra.Command{
	Use:   "talk",
	Short: "Talks to a Yamamaha Musiccast device",
	RunE: func(cmd *cobra.Command, args []string) error {
		devices, err := musiccast.Discover()
		if err != nil {
			return err
		}
		if len(devices) == 0 {
			return fmt.Errorf("no devices found")
		}

		dev := devices[0]
		err = dev.PowerOn()
		if err != nil {
			return err
		}

		err = dev.PowerOff()
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(talkCmd)
}
