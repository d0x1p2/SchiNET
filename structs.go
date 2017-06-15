package main

import (
	"sync"

	"gopkg.in/mgo.v2/bson"

	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/d0x1p2/godbot"
)

// Bot is a wrapper for the godbot.Core
type bot struct {
	mu sync.Mutex
	*godbot.Core
}

// IOdat is input/output processed.
type IOdat struct {
	//err  error // Tracking errors.
	help   bool // If HELP is in the input
	rm     bool // Remove initial message.
	io     []string
	input  string
	output string

	user     godbot.User
	guild    *godbot.Guild
	msg      *discordgo.MessageCreate
	msgEmbed *discordgo.MessageEmbed
}

// DBdat passes information as to what to store into a database.
type DBdat struct {
	Database   string
	Collection string
	Document   interface{}
	Documents  []interface{}
	Query      bson.M
	Change     bson.M
}

// Event has information regarding upcoming events.
type Event struct {
	ID          bson.ObjectId `bson:"_id,omitempty"`
	Description string
	Day         string
	Time        time.Time
	Protected   bool
	AddedBy     godbot.User
}

// Command structure for User Defined commands.
type Command struct {
	ID bson.ObjectId `bson:"_id,omitempty"`
}
