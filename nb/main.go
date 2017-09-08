package main

import (
	//"fmt"
	"os"

	"github.com/paulstuart/notion"
)

func readme() {
	f, err := os.Open("todolist.txt")
	if err != nil {
		panic(err)
	}
	s := testSession()
	if err := s.Import(f); err != nil {
		panic(err)
	}
	if err := s.TextAppend(4, " real soon"); err != nil {
		panic(err)
	}
	if err := s.TextDelete(7, 7, " ant"); err != nil {
		panic(err)
	}
	if err := s.TextInsert(5, 1, "ave s"); err != nil {
		panic(err)
	}
	//s.NotionJSON(os.Stdout)
	s.Print(os.Stdout)
	os.Exit(0)
}

func testSession() *notion.Session {
	me := &notion.Person{
		First: "Paul",
		Last:  "Stuart",
	}
	l := notion.TestLogger(os.Stderr)
	n, err := notion.NewNotion("whatever", nil, l)
	if err != nil {
		panic(err)
	}
	s, err := n.Login(nil, me)
	if err != nil {
		panic(err)
	}
	return s
}

func main() {
	readme()

	me := &notion.Person{
		First: "Paul",
		Last:  "Stuart",
	}
	//fmt.Println("here we go!")
	l := notion.TestLogger(os.Stdout)
	n, err := notion.NewNotion("ok, whatever", nil, l)
	if err != nil {
		panic(err)
	}
	s, err := n.Login(nil, me)
	if err != nil {
		panic(err)
	}
	first, err := s.ItemAppend(&n.Root, "line one")
	if err != nil {
		panic(err)
	}
	if _, err := s.ItemAppend(&n.Root, "number 2"); err != nil {
		panic(err)
	}
	if _, err := s.ItemAppend(&n.Root, "third time"); err != nil {
		panic(err)
	}
	if _, err := s.ItemAppend(first, "first child"); err != nil {
		panic(err)
	}

	n.Print(os.Stdout, 1)
	s.NotionJSON(os.Stdout)
}
