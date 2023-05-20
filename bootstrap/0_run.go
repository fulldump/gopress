package bootstrap

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func Run(runners ...Runner) {
	RunUntilSignal(RunAll(runners...), syscall.SIGTERM, syscall.SIGINT)
}

func RunAll(runners ...Runner) Runner {
	return func() (start func() error, stop func() error) {

		wg := sync.WaitGroup{}
		stops := []func() error{}

		start = func() error {

			for i, run := range runners {
				wg.Add(1)

				start, stop := run()

				stops = append(stops, stop)

				go func(i int, start func() error) {
					// TODO: handle panics
					defer wg.Done()
					// fmt.Printf("Task %d: start\n", i)
					err := start() // blocking call
					// if err != nil {
					// 	fmt.Printf("Task %d: error: %s\n", i, err.Error())
					// } else {
					// 	fmt.Printf("Task %d: started\n", i)
					// }
					if err != nil {
						log.Println("start:", err.Error())
					}
				}(i, start)
			}

			wg.Wait()

			return nil
		}

		stop = func() error {
			for i, stop := range stops {
				wg.Add(1)
				go func(i int, stop func() error) {
					defer wg.Done()
					// fmt.Printf("Task %d: stopping...\n", i)
					err := stop()
					if err != nil {
						log.Println("stop:", err.Error())
						// fmt.Printf("Task %d: error: %s\n", i, err.Error())
					}
				}(i, stop)
			}

			fmt.Println("Waiting for all runners to stop...")

			return nil
		}

		return start, stop
	}
}

func RunUntilSignal(run Runner, s ...os.Signal) {
	start, stop := run()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, s...)
	go func() {
		<-sigs
		if err := stop(); err != nil {
			fmt.Println("ERROR stop:", err.Error())
		}

		// fmt.Println("signal again to kill")
		// <-sigs
		// os.Exit(-1)
	}()

	err := start()
	if err != nil {
		fmt.Println("ERROR start:", err.Error())
	}
}
