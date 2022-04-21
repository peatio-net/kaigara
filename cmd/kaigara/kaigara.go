package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/openware/kaigara/pkg/config"
	"github.com/openware/kaigara/pkg/env"
	"github.com/openware/kaigara/pkg/logstream"
	"github.com/openware/kaigara/pkg/storage"
	"github.com/openware/kaigara/types"
	"github.com/openware/pkg/ika"
)

var conf = &config.KaigaraConfig{}
var ls logstream.LogStream

func parseScopes() []string {
	return strings.Split(conf.Scopes, ",")
}

func parseAppNames() []string {
	return strings.Split(conf.AppNames, ",")
}

func appNamesToLoggingName() string {
	return strings.Join(parseAppNames(), "&")
}

func kaigaraRun(ss types.Storage, cmd string, cmdArgs []string) {
	log.Printf("INF: starting command: %s %v\n", cmd, cmdArgs)
	scopes := parseScopes()
	c := exec.Command(cmd, cmdArgs...)
	envs, err := env.BuildCmdEnv(parseAppNames(), ss, os.Environ(), scopes)
	if err != nil {
		panic(err)
	}

	c.Env = envs.Vars

	for _, file := range envs.Files {
		err := os.MkdirAll(path.Dir(file.Path), 0750)
		if err != nil {
			panic(fmt.Sprintf("Failed to make dir %s: %s", file.Path, err.Error()))
		}

		err = ioutil.WriteFile(file.Path, []byte(file.Content), 0640)
		if err != nil {
			panic(fmt.Sprintf("Failed to write file %s: %s", file.Path, err.Error()))
		}
	}

	stdin, err := c.StdinPipe()
	if err != nil {
		panic(err)
	}

	// Read STDIN and write it to the command
	go func() {
		r := bufio.NewReader(os.Stdin)
		for {
			line, isPrefix, err := r.ReadLine()
			if err == io.EOF {
				log.Printf("INF: reached EOF on STDIN\n")
				stdin.Close()
				break
			} else if err != nil {
				panic(err)
			}
			_, err = stdin.Write(line)
			if err != nil {
				panic(err)
			}

			if !isPrefix {
				_, err = stdin.Write([]byte("\n"))
				if err != nil {
					panic(err)
				}
			}
		}
	}()

	stdout, err := c.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}

	stderr, err := c.StderrPipe()
	if err != nil {
		log.Fatal(err)
	}

	channelOut := fmt.Sprintf("log.%s.%s", appNamesToLoggingName(), "stdout")
	channelErr := fmt.Sprintf("log.%s.%s", appNamesToLoggingName(), "stderr")
	log.Printf("INF: publishing on %s and %s\n", channelOut, channelErr)

	var wg sync.WaitGroup
	if ls != nil {
		wg.Add(2)

		go func() {
			if err := ls.Publish(channelOut, stdout); err != nil {
				log.Printf("ERR: %s", err.Error())
			}
			wg.Done()
		}()

		go func() {
			if err := ls.Publish(channelErr, stderr); err != nil {
				log.Printf("ERR: %s", err.Error())
			}
			wg.Done()
		}()
	}

	if err := c.Start(); err != nil {
		log.Fatal(err)
	}

	go exitWhenSecretsOutdated(c, ss, scopes)

	quit := make(chan int)
	if ls != nil {
		wg.Add(1)
		go func() {
			ls.HeartBeat(appNamesToLoggingName(), quit)
			wg.Done()
		}()
	}

	if err := c.Wait(); err != nil {
		log.Fatal(err)
	}

	if ls != nil {
		quit <- 0
	}

	wg.Wait()
}

func exitWhenSecretsOutdated(c *exec.Cmd, ss types.Storage, scopes []string) {
	appNames := append(parseAppNames(), "global")

	if ignore, ok := os.LookupEnv("KAIGARA_IGNORE_GLOBAL"); ok && ignore == "true" {
		appNames = appNames[:len(appNames)-1]
	}

	for range time.Tick(time.Second * 20) {
		for _, appName := range appNames {
			for _, scope := range scopes {
				current, err := ss.GetCurrentVersion(appName, scope)
				if err != nil {
					log.Println(err.Error())
					break
				}
				latest, err := ss.GetLatestVersion(appName, scope)
				if err != nil {
					log.Println(err.Error())
					break
				}
				if current != latest {
					log.Printf("INF: found secrets updated on '%v' scope. from: v%v, to: v%v. killing process...\n", scope, current, latest)
					if err := c.Process.Kill(); err != nil {
						log.Fatalf("FTL: failed to kill process: %s", err.Error())
					}
				}
			}
		}
	}
}

func main() {
	log.SetPrefix("[Kaigara] ")
	if len(os.Args) < 2 {
		panic("Usage: kaigara CMD [ARGS...]")
	}

	if err := ika.ReadConfig("", conf); err != nil {
		panic(err)
	}

	var err error
	ls, err = logstream.NewRedisClient(conf.RedisURL)
	if err != nil {
		log.Printf("WRN: %s", err.Error())
		ls = nil
	}

	ss, err := storage.GetStorageService(conf)
	if err != nil {
		panic(err)
	}

	kaigaraRun(ss, os.Args[1], os.Args[2:])
}
