package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"time"
)

func main() {
	log := log.New(os.Stderr, "", 0)
	flag.Parse()
	if flag.NArg() < 3 {
		log.Fatalln("Usage: migrate create <dir> <name>")
	}

	var err error
	switch flag.Arg(0) {
	case "create":
		err = create(flag.Arg(1), flag.Arg(2))
	default:
		err = errors.New("unknown command " + flag.Arg(0))
	}
	if err != nil {
		log.Fatalln("Error:", err)
	}
}

func create(dir, name string) error {
	now := time.Now().Unix()
	prefix := fmt.Sprintf("%v-%v", now, name)
	for _, suffix := range []string{".up.sql", ".down.sql"} {
		f, err := os.Create(path.Join(dir, prefix+suffix))
		if err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}
