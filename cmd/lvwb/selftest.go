package main

import (
	"errors"
	"fmt"

	"github.com/Gitguy268/LvW-bench/internal/score"
	"github.com/Gitguy268/LvW-bench/internal/tasks"
)

func cmdSelftest() error {
	fails, n := score.SelfTest()
	for _, f := range fails {
		fmt.Println("FAIL", f)
	}
	items := 0
	for _, c := range tasks.Configs {
		if c.Kind == "gen" {
			xs, err := tasks.GenItems(c.Name, 0)
			if err != nil {
				return err
			}
			items += len(xs)
		} else {
			xs, err := tasks.DecideItems(c.Name, "test")
			if err != nil {
				return err
			}
			items += len(xs)
		}
	}
	if len(fails) > 0 {
		return errors.New("scorer self-test failed")
	}
	fmt.Printf("ok: %d scorer checks passed, %d task items load\n", n, items)
	return nil
}
