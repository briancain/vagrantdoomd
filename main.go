package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	CONN_HOST = "0.0.0.0"
	CONN_PORT = "8888"
	CONN_TYPE = "tcp"
)

func runCmd(cmdstring string) {
	parts := strings.Split(cmdstring, " ")
	cmd := exec.Command(parts[0], parts[1:len(parts)]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Fatalf("The following command failed: \"%v\"\n", cmdstring)
	}
}

func outputCmd(cmdstring string) string {
	parts := strings.Split(cmdstring, " ")
	cmd := exec.Command(parts[0], parts[1:len(parts)]...)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		log.Fatalf("The following command failed: \"%v\"\n", cmdstring)
	}
	return string(output)
}

func startCmd(cmdstring string) {
	parts := strings.Split(cmdstring, " ")
	cmd := exec.Command(parts[0], parts[1:len(parts)]...)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	err := cmd.Start()
	if err != nil {
		log.Fatalf("The following command failed: \"%v\"\n", cmdstring)
	}
}

func checkDockerImages(imageName, dockerBinary string) bool {
	output := outputCmd(fmt.Sprintf("%v images -q %v", dockerBinary, imageName))
	return len(output) > 0
}

func checkActiveDocker(dockerName, dockerBinary string) bool {
	return checkDocker(dockerName, dockerBinary, "-q")
}

func checkAllDocker(dockerName, dockerBinary string) bool {
	return checkDocker(dockerName, dockerBinary, "-aq")
}

func checkDocker(dockerName, dockerBinary, arg string) bool {
	output := outputCmd(fmt.Sprintf("%v ps %v", dockerBinary, arg))
	docker_ids := strings.Split(string(output), "\n")
	for _, docker_id := range docker_ids {
		if len(docker_id) == 0 {
			continue
		}
		output := outputCmd(fmt.Sprintf("%v inspect -f {{.Name}} %v", dockerBinary, docker_id))
		name := strings.TrimSpace(string(output))
		name = name[1:len(name)]
		if name == dockerName {
			return true
		}
	}
	return false
}

func startServer(listener net.Listener, dockerBinary, containerName string) {
	log.Println("Starting server...")
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	go func(listener net.Listener, c chan os.Signal) {
		sig := <-c
		log.Printf("Caught signal %s: shutting down.", sig)
		listener.Close()
		os.Exit(0)
	}(listener, sigc)

	for {
		log.Println("About to listen")
		conn, err := listener.Accept()
		log.Println("Made it past listen")
		if err != nil {
			log.Fatalf("There was an error after accepting the listening: ", err)
			os.Exit(1)
		}

		socketLoop(conn, dockerBinary, containerName)
	}
}

func socketLoop(conn net.Conn, dockerBinary, containerName string) {
	stop := false
	for !stop {
		bytes := make([]byte, 40960)
		n, err := conn.Read(bytes)
		if err != nil {
			stop = true
		}
		bytes = bytes[0:n]
		strbytes := strings.TrimSpace(string(bytes))
		log.Print(strbytes)
		if strbytes == "list" {
			output := outputCmd(fmt.Sprintf("%v ps -q", dockerBinary))
			//cmd := exec.Command("/usr/bin/docker", "inspect", "-f", "{{.Name}}", "`docker", "ps", "-q`")
			outputstr := strings.TrimSpace(output)
			outputparts := strings.Split(outputstr, "\n")
			for _, part := range outputparts {
				output := outputCmd(fmt.Sprintf("%v inspect -f {{.Name}} %v", dockerBinary, part))
				name := strings.TrimSpace(output)
				name = name[1:len(name)]
				log.Printf("container name: %s", name)
				if name != containerName {
					_, err = conn.Write([]byte(name + "\n"))
					if err != nil {
						log.Fatal("Could not write to socker file")
					}
				}
			}
			conn.Close()
			stop = true
		} else if strings.HasPrefix(strbytes, "kill ") {
			parts := strings.Split(strbytes, " ")
			docker_id := strings.TrimSpace(parts[1])
			cmd := exec.Command(dockerBinary, "rm", "-f", docker_id)
			go cmd.Run()
			conn.Close()
			stop = true
		}
	}
}

func cleanup() {

}

func createSocket(socketFileFormat string) (net.Listener, string) {
	socketFile := fmt.Sprintf(socketFileFormat, time.Now().Unix())
	listener, err := net.Listen("unix", socketFile)
	if err != nil {
		log.Fatalf("Could not create socket file %v.\nYou could use \"rm -f %v\"", socketFile, socketFile)
	} else {
		log.Printf("Created socket file at: %v", socketFile)
	}
	return listener, socketFile
}

func createTCP() net.Listener {
	listener, err := net.Listen(CONN_TYPE, CONN_HOST+":"+CONN_PORT)
	if err != nil {
		log.Fatalf("Could not create %v server at %v:%v %v", CONN_TYPE, CONN_HOST, CONN_PORT, err.Error())
	}

	//defer listener.Close()
	return listener
}

func main() {
	var socketFileFormat, containerName, vncPort, dockerBinary, imageName, dockerfile string
	var dockerWait int
	var buildImage, asciiDisplay bool
	flag.StringVar(&socketFileFormat, "socketFileFormat", "/tmp/dockerdoom%v.socket", "Location and format of the socket file")
	flag.StringVar(&containerName, "containerName", "dockerdoom", "Name of the docker container running DOOM")
	flag.IntVar(&dockerWait, "dockerWait", 5, "Time to wait before checking if the container came up")
	flag.StringVar(&vncPort, "vncPort", "5900", "Port to open for VNC Viewer")
	flag.StringVar(&dockerBinary, "dockerBinary", "/usr/bin/docker", "docker binary")
	flag.BoolVar(&buildImage, "buildImage", false, "Build docker image instead of pulling it from docker image repo")
	flag.StringVar(&imageName, "imageName", "briancain/dockerdoom", "Name of docker image to use")
	flag.StringVar(&dockerfile, "dockerfile", ".", "Path to dockerdoom's Dockerfile")
	flag.BoolVar(&asciiDisplay, "asciiDisplay", false, "Don't use fancy vnc, throw DOOM straightup on my terminal screen")
	flag.Parse()

	if buildImage {
		log.Print("Building dockerdoom image, this will take a few minutes...")
		runCmd(fmt.Sprintf("%v build -t %v %v", dockerBinary, imageName, dockerfile))
		log.Print("Image has been built")
	}
	present := checkDockerImages(imageName, dockerBinary)
	if !present {
		log.Print("Pulling image from public repo")
		runCmd(fmt.Sprintf("%v pull %v", dockerBinary, imageName))
		log.Print("Image downloaded")
	}

	present = checkAllDocker(containerName, dockerBinary)
	if present {
		log.Fatalf("\"%v\" was present in the output of \"docker ps -a\",\nplease remove before trying again. You could use \"docker rm -f %v\"\n", containerName, containerName)
	}

	//listener,socketFile := createSocket(socketFileFormat)
	listener := createTCP()

	log.Print("Trying to start docker container ...")
	if !asciiDisplay {
		//dockerRun := fmt.Sprintf("%v run --rm=true -p %v:%v -v %v:/dockerdoom.socket --name=%v %v x11vnc -geometry 640x480 -forever -usepw -create", dockerBinary, vncPort, vncPort, socketFile, containerName, imageName)
		dockerRun := fmt.Sprintf("%v run --rm=true -it --net=bridge -p %v:%v --name=%v %v x11vnc -geometry 640x480 -forever -usepw -create", dockerBinary, vncPort, vncPort, containerName, imageName)
		log.Printf("Running docker command: %v", dockerRun)
		startCmd(dockerRun)
		log.Printf("Waiting %v seconds for \"%v\" to show in \"docker ps\". You can change this wait with -dockerWait.", dockerWait, containerName)
		time.Sleep(time.Duration(dockerWait) * time.Second)
		present = checkActiveDocker(containerName, dockerBinary)
		if !present {
			log.Fatalf("\"%v\" did not lead to the container appearing in \"docker ps\". Please try and start it manually and check \"docker ps\"\n", dockerRun)
		}
		log.Print("Docker container started, you can now connect to it with a VNC viewer at port 5900")
	} else {
		dockerRun := fmt.Sprintf("%v run -it --rm=true -p %v:%v p 8888 --name=%v %v /bin/bash", dockerBinary, vncPort, vncPort, containerName, imageName)
		startCmd(dockerRun)
	}

	startServer(listener, dockerBinary, containerName)
}
