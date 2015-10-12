package main

import (
	"log"

	"github.com/jandre/dockerpc"
)

func main() {

	image := "docker-plugin:latest"
	host := "tcp://192.168.99.100:2376"

	client := dockerpc.NewClient("XXX-test", image, host)

	err := client.Start()
	defer client.Close()

	if err != nil {
		log.Fatal(err)
	}

	name := "jen"
	var result string
	err = client.Call("Plugin.SayHi", name, &result)

	log.Print(client.StdError())
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Plugin.SayHi", name, "Returned:", result)
	name = "bob"
	err = client.Call("Plugin.SayHi", name, &result)

	log.Print(client.StdError())
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Plugin.SayHi", name, "Returned:", result)

	// this should fail
	err = client.Call("Plugin.SayHi2", name, &result)

	log.Print(client.StdError())
	if err != nil {
		log.Println("Error:", err)
	} else {
		log.Println("Plugin.SayHi2", name, "Returned:", result)
	}
}
