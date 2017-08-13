package main

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/pborman/getopt/v2"

	"github.com/d0x1p2/godbot"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// Flags that can be parsed related to User commands.
type userFlags struct {
	flag       *getopt.Set // FlagSet object -> used for Help
	server     string      // Server for opperation.
	serverName string      // Name of the server.
	// Generics
	User    string // User to perform action on.
	Comment string // Comment for action.
	Type    string // Type of Action.
	Help    bool   // Command Help
	List    bool   // List Command objects/items/etc
	Add     bool
	Remove  bool

	// Ban related
	Ban bool // Ban enabled.

	// Credit related
	Xfer   bool // Transfer
	Gamble bool // Gamble
	Amount int  // Amount of Credits for action.
	All    bool // All credits.

	// Permission related
	Permission bool
}

// Error constants.
var (
	ErrBadUser        = errors.New("bad user supplied")
	ErrBadPermissions = errors.New("you do not have permissions to do that")
	ErrBanChanExists  = errors.New("user already has a ban for that channel")
	ErrBanNotFound    = errors.New("ban not found to clear")

	banSyntaxHard   = ",ban  --type hard  --user \"@Username\"\n"
	banSyntaxSoft   = ",ban  --type soft  --user \"@Username\"   -c \"Bug Exploits\"\n"
	banSyntaxRemove = ",ban  --type hard  --user \"@Username\"   --remove\n"
	banSyntaxAll    = banSyntaxHard + banSyntaxSoft + banSyntaxRemove

	permSyntaxAdd    = ",permission   --add       --type \"Permission\"   --user \"@Username\"\n"
	permSyntaxRemove = ",permission   --remove    --type \"Permission\"   --user \"@Username\"\n"
	permSyntaxAll    = "\n\n" + permSyntaxAdd + permSyntaxRemove

	userSyntaxUser       = ",user   --user \"@Username\"\n"
	userSyntaxGamble     = ",user   --gamble   -n 100\n"
	userSyntaxBan        = ",user   --ban   --type soft   --user \"@Username\"\n"
	userSyntaxXfer       = ",user   -x   --user \"@Username\"   -n 100\n"
	userSyntaxPermission = ",user   --permission   --add   --type \"mod\"   --user \"@Username\"\n"
	userSyntaxAll        = "\n\n" + userSyntaxUser + userSyntaxBan + userSyntaxGamble + userSyntaxPermission + userSyntaxXfer
)

// Permission scheme constants.
const (
	permNormal = 1 << iota
	permModerator
	permAdmin
	permAscended
)

const permAll = permAdmin | permModerator | permAscended | permNormal

// CoreUser processes all user-related commands.
func (io *IOdat) CoreUser() error {
	u := io.user
	var uflags userFlags
	var all bool

	for n, s := range io.io {
		if s == "-n" {
			if n+1 <= len(io.io)-1 {
				if io.io[n+1] == "all" {
					io.io = append(io.io[:n], io.io[n+2:]...)
					all = true
					uflags.All = true
				} else {
					break
				}
			} else {
				io.io = append(io.io, "0")
			}
		}
	}

	fl := getopt.New()

	// Generics
	fl.FlagLong(&uflags.User, "user", 0, "Username")
	fl.FlagLong(&uflags.Comment, "comment", 'c', "Ban Comment")
	fl.FlagLong(&uflags.Type, "type", 't', "Type")
	fl.FlagLong(&uflags.Help, "help", 'h', "This message")
	fl.FlagLong(&uflags.List, "list", 0, "List all bans.")
	fl.FlagLong(&uflags.Add, "add", 0, "Add")
	fl.FlagLong(&uflags.Remove, "remove", 0, "Remove")

	// Ban related.
	fl.FlagLong(&uflags.Ban, "ban", 0, "Ban a user.")

	// Gambling related.
	fl.FlagLong(&uflags.Xfer, "xfer", 'x', "Xfer credits")
	fl.FlagLong(&uflags.Gamble, "gamble", 'g', "Gamble")
	if !all {
		fl.Flag(&uflags.Amount, 'n', "Amount (Number)")
		fl.FlagLong(&uflags.All, "all", 0, "Gamble all Credits")
	}

	// Permission related.
	fl.FlagLong(&uflags.Permission, "permission", 'p', "Permission Modification")

	if err := fl.Getopt(io.io, nil); err != nil {
		return err
	}
	if fl.NArgs() > 0 {
		if err := fl.Getopt(fl.Args(), nil); err != nil {
			return err
		}
	}

	uflags.flag = fl
	uflags.User = userIDClean(uflags.User)
	uflags.server = io.guild.ID

	var msg string
	var err error
	switch {
	case uflags.Ban:
		msg, err = u.Ban(io.msg.ChannelID, uflags)
	case uflags.Xfer:
		msg, err = u.Transfer(uflags.Amount, uflags.User)
	case uflags.Gamble:
		if uflags.All {
			uflags.Amount = u.Credits
		} else if len(io.io) < 4 {
			return ErrBadArgs
		}
		msg, err = u.Gamble(uflags.Amount)
	case uflags.Permission:
		msg, err = u.Permission(uflags)
	default:
		if uflags.Help {
			io.output = Help(fl, "", userSyntaxAll)
			return nil
		} else if uflags.User != "" {
			// Get user information
			user := UserNew(nil)
			if err := user.Get(uflags.User); err != nil {
				return err
			}
			io.msgEmbed = user.EmbedCreate()
			return nil
		} else {
			// Return current User
			io.msgEmbed = u.EmbedCreate()
			return nil
		}
	}

	if err != nil {
		return err
	}

	io.msgEmbed = embedCreator(msg, ColorBlue)
	return nil
}

// UserNew creates a new user instance based on accessing user.
func UserNew(u *discordgo.User) *User {
	if u == nil {
		u = &discordgo.User{
			ID:            "",
			Username:      "",
			Discriminator: "",
			Bot:           false,
		}
	}
	var user = User{
		ID:            u.ID,
		Username:      u.Username,
		Discriminator: u.Discriminator,
		Bot:           u.Bot,
		CreditsTotal:  1,
		Credits:       1,
	}
	return &user
}

// UserUpdateSimple stream-lines the process for incrementing credits.
func UserUpdateSimple(serverID string, user *discordgo.User, inc int, ts time.Time) error {
	u := UserNew(user)

	if err := u.Get(u.ID); err != nil {
		if err == mgo.ErrNotFound {
			u.Credits = 0
			u.CreditsTotal = 0
			u.LastSeen = ts
			u.Access = append(u.Access, Access{ServerID: serverID, Permissions: permNormal})
			if err := u.Update(); err != nil {
				return err
			}
			return nil
		}
		return err
	}

	if inc == 1 {
		u.CreditsTotal++
		u.Credits++
	} else {
		u.Credits += inc
	}

	// Make sure to only add most recent messages in time.
	if ts.After(u.LastSeen) {
		u.LastSeen = ts
	}

	if err := u.Update(); err != nil {
		return err
	}

	return nil

}

// Update pushes an update to the database.
func (u *User) Update() error {
	var err error
	var q = make(map[string]interface{})
	var c = make(map[string]interface{})

	q["id"] = u.ID
	c["$set"] = bson.M{
		"username":     u.Username,
		"creditstotal": u.CreditsTotal,
		"credits":      u.Credits,
		"access":       u.Access,
		"lastseen":     u.LastSeen,
	}

	var dbdat = DBdatCreate(Database, CollectionUsers, u, q, c)
	err = dbdat.dbEdit(User{})
	if err != nil {
		if err == mgo.ErrNotFound {
			// Add to DB since it doesn't exist.
			if err := dbdat.dbInsert(); err != nil {
				return err
			}
			return nil
		}
		return err
	}

	return nil
}

// Get a user from a database
func (u *User) Get(uID string) error {
	var q = make(map[string]interface{})

	q["id"] = uID

	var dbdat = DBdatCreate(Database, CollectionUsers, User{}, q, nil)
	err := dbdat.dbGet(User{})
	if err != nil {
		return err
	}

	var user = User{}
	user = dbdat.Document.(User)
	*u = user

	return nil
}

// GetByName from database.
func (u *User) GetByName(username string) error {
	var q = make(map[string]interface{})

	q["username"] = username

	var dbdat = DBdatCreate(Database, CollectionUsers, User{}, q, nil)
	err := dbdat.dbGet(User{})
	if err != nil {
		return err
	}

	var user = User{}
	user = dbdat.Document.(User)
	*u = user

	return nil
}

// EmbedCreate returns an embed object with User information.
func (u *User) EmbedCreate() *discordgo.MessageEmbed {
	tn := time.Now()
	dur := tn.Sub(u.LastSeen)

	ta := fmt.Sprintf("**%d** hours, **%d** minutes.", int(dur.Hours()), int(dur.Minutes())%60)

	description := fmt.Sprintf(
		"__ID: %s__\n"+
			"**Username**:   %-18s **%s**: %-10d\n"+
			"**Reputation**: %d\n\n"+
			"**Last Seen**: %s",
		u.ID,
		fmt.Sprintf("%s#%s", u.Username, u.Discriminator), strings.Title(GambleCredits), u.Credits,
		u.CreditsTotal,
		ta)

	return &discordgo.MessageEmbed{
		Author:      &discordgo.MessageEmbedAuthor{},
		Color:       ColorBlue,
		Description: description,
		Fields:      []*discordgo.MessageEmbedField{},
	}
}

// String produces a Username#Discriminator string.
func (u *User) String() string {
	return u.Username + "#" + u.Discriminator
}

// StringPretty produces elegant looking string.
func (u *User) StringPretty() string {
	return "**" + u.Username + "**#" + u.Discriminator
}

// Basic provides a basic User structure.
func (u *User) Basic() UserBasic {
	return UserBasic{
		ID:            u.ID,
		Name:          u.Username,
		Discriminator: u.Discriminator,
	}
}

// String returns a basic string of the object.
func (ub UserBasic) String() string {
	return ub.Name + "#" + ub.Discriminator
}

// StringPretty makes everything prettier.
func (ub UserBasic) StringPretty() string {
	return "**" + ub.Name + "**#" + ub.Discriminator
}

/*
	BAN RELATED
	ACTIONS
*/

// Ban will remove a player from the server
func (u *User) Ban(cID string, fl userFlags) (string, error) {

	var err error
	var uID, msg string

	// Properly format ID string given by the person performing the ban.
	if fl.User != "" {
		ids := strings.FieldsFunc(fl.User, idSplit)
		uID = ids[0]
	}

	if u.ID == uID {
		return fmt.Sprintf("Oh c'mon <@%s>m that would just be silly...", u.ID), nil
	}

	if err = u.Get(u.ID); err != nil {
		return "", err
	}

	if ok := u.HasPermission(fl.server, permAdmin); !ok {
		return "", ErrBadPermissions
	} else if fl.Help {
		prefix := "**Need** __type__ and __username__.\n\n"
		return Help(fl.flag, prefix, banSyntaxAll), nil
	} else if fl.List {
		return banList(fl.serverName, fl.server)
	}

	if (fl.Type == "soft" || fl.Type == "hard") && (uID != "") {
		// Find user.
		criminal := UserNew(nil)
		if err = criminal.Get(uID); err != nil {
			return "", err
		}

		// Create a new ban.
		var b = criminal.BanNew()
		b.ByLast = &UserBasic{ID: u.ID, Name: u.Username, Discriminator: u.Discriminator}

		if fl.Type == "soft" {
			if msg, err = b.banSoft(fl.server, cID, fl.Comment, fl.Remove); err != nil {
				return "", err
			}
		} else if fl.Type == "hard" {
			if msg, err = b.banHard(fl.server, cID, fl.Comment, fl.Remove); err != nil {
				return "", err
			}
		}
	} else if fl.Type == "bot" && uID != "" {
		criminal := UserNew(nil)
		if err = criminal.Get(uID); err != nil {
			return "", err
		}

		msg = fmt.Sprintf("Bot access has been __**revoked**__ for <@%s>.", criminal.ID)
		criminal.PermissionSet(fl.server, 0)
		if fl.Remove {
			criminal.PermissionAdd(fl.server, permNormal)
			msg = fmt.Sprintf("Bot access has been __**restored**__ for <@%s>.", criminal.ID)
		} else {
			var b = criminal.BanNew()
			if err := b.Get(fl.server); err != nil {
				if err != mgo.ErrNotFound {
					return "", err
				}
			}
			b.Amount++
			if err := b.Update(fl.server); err != nil {
				return "", err
			}
		}
		if err := criminal.Update(); err != nil {
			return "", err
		}
		return msg, nil

	} else {
		prefix := "**Need** __type__ and __username__.\n\n"
		return Help(fl.flag, prefix, banSyntaxAll), nil
	}

	return msg, nil

}

func (b *Ban) banSoft(database, cID, comment string, remove bool) (string, error) {
	var msg string
	if err := b.Get(database); err != nil {
		if err == mgo.ErrNotFound {
			if err := b.Update(database); err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	if !remove {
		for _, c := range b.Channels {
			if c.ChannelID == cID {
				return "", ErrBanChanExists
			}
		}

		c := Bot.GetChannel(cID)

		cNew := chanBan{
			Name:      c.Name,
			ChannelID: c.ID,
			Comment:   comment,
			By:        b.ByLast,
			Date:      time.Now(),
		}

		b.Amount++
		b.Channels = append(b.Channels, cNew)
		b.Last = time.Now()
		b.ByLast = nil

		// Attempt permission changes here.
		err := Bot.Session.ChannelPermissionSet(cID, b.User.ID, "member", 0, 2048)
		if err != nil {
			return "", err
		}

		if err := b.Update(database); err != nil {
			return "", err
		}

		msg = fmt.Sprintf("<@%s> has **soft banned** <@%s> from the <#%s> channel.\n",
			cNew.By.ID, b.User.ID, cID)

	} else {
		var found bool
		var adminID string
		for n, c := range b.Channels {
			if c.ChannelID == cID {
				found = true
				copy(b.Channels[n:], b.Channels[n+1:])
				b.Channels[len(b.Channels)-1] = chanBan{}
				b.Channels = b.Channels[:len(b.Channels)-1]
				adminID = c.By.ID
			}
		}
		if !found {
			return "", ErrBanNotFound
		}
		if err := Bot.Session.ChannelPermissionDelete(cID, b.User.ID); err != nil {
			return "", err
		}

		if err := b.Update(database); err != nil {
			return "", err
		}

		msg = fmt.Sprintf("<@%s> has **removed** <@%s>'s soft ban for the <#%s> channel.\n",
			adminID, b.User.ID, cID)
	}

	return msg, nil
}

func (b *Ban) banHard(database, cID, comment string, remove bool) (string, error) {
	var msg string
	if err := b.Get(database); err != nil {
		if err == ErrScriptNotFound {
			if err := b.Update(database); err != nil {
				return "", err
			}
		}
		return "", err
	}

	gID, err := Bot.GetGuildID(cID)
	if err == godbot.ErrNotFound {
		return "", errors.New("Seems something happened getting Guild ID")
	}

	if !remove {
		if err := Bot.Session.GuildBanCreateWithReason(gID, b.User.ID, comment, 1); err != nil {
			return "", err
		}
		msg = fmt.Sprintf("<@%s> has **banned** %s from the server.\n Sucks to suck.",
			b.ByLast.ID, b.User.Name)
		return msg, nil
	}

	if err := Bot.Session.GuildBanDelete(gID, b.User.ID); err != nil {
		return "", err
	}
	msg = fmt.Sprintf("<@%s> has **banned** %s from the server.\n Sucks to suck.",
		b.ByLast.ID, b.User.Name)

	return msg, nil
}

// BanNew creates a new instance of a Ban.
func (u *User) BanNew() *Ban {
	var ub = &UserBasic{
		ID:            u.ID,
		Name:          u.Username,
		Discriminator: u.Discriminator,
	}
	return &Ban{
		User:   ub,
		ByLast: nil,
		Amount: 0,
	}
}

// Get a ban object from Database.
func (b *Ban) Get(database string) error {
	var q = make(map[string]interface{})
	q["user.id"] = b.User.ID

	dbdat := DBdatCreate(database, CollectionBlacklist, Ban{}, q, nil)
	err := dbdat.dbGet(Ban{})
	if err != nil {
		if err == mgo.ErrNotFound {
			return err
		}
		return err
	}

	var ban Ban
	ban = dbdat.Document.(Ban)
	if len(ban.Channels) > 0 {
		for _, c := range ban.Channels {
			b.Channels = append(b.Channels, c)
		}
	}

	b.User = ban.User
	b.Amount = ban.Amount
	b.Last = ban.Last

	return nil
}

// Update database with new information.
func (b *Ban) Update(database string) error {
	var q = make(map[string]interface{})
	var c = make(map[string]interface{})
	q["user.id"] = b.User.ID
	c["$set"] = bson.M{
		"channels": b.Channels,
		"amount":   b.Amount,
		"bylast":   nil,
		"last":     b.Last,
	}

	dbdat := DBdatCreate(database, CollectionBlacklist, b, q, c)
	err := dbdat.dbEdit(Ban{})
	if err != nil {
		if err == mgo.ErrNotFound {
			if err := dbdat.dbInsert(); err != nil {
				return err
			}
			return nil
		}
		return err
	}

	return nil
}

func banList(database, id string) (string, error) {
	var msg string

	db := DBdatCreate(database, CollectionBlacklist, Ban{}, nil, nil)
	if err := db.dbGetAll(Ban{}); err != nil {
		return "", err
	}

	msg = "List of current Soft Bans:\n\n"
	// Convert here.
	if len(db.Documents) == 0 {
		return "List is empty.", nil
	}

	var b Ban
	var found bool
	for _, criminal := range db.Documents {
		var botban bool
		b = criminal.(Ban)
		u := UserNew(nil)
		if err := u.Get(b.User.ID); err != nil {
			fmt.Println("Could not get user while getting ban list", err.Error())
		}
		if !u.HasPermission(id, permNormal) {
			botban = true
		}

		if len(b.Channels) > 0 || botban {
			found = true
			msg += fmt.Sprintf("\t__**%s**#%s__\n", b.User.Name, b.User.Discriminator)
			if botban {
				msg += "\t\t**Bot Banned.**\n"
			}
			for _, c := range b.Channels {
				msg += fmt.Sprintf("\t\tChannel: **%s**, Comment: **%s**\n", c.Name, c.Comment)
			}
		}
	}

	if !found {
		return "List is empty.", nil
	}
	return msg, nil
}

/*
	GAMBLE RELATED
	ACTIONS
*/

// Gamble User Credits.
func (u *User) Gamble(amount int) (string, error) {
	var twealth, spoils int
	var err error

	if amount < 50 {
		return "", ErrGambleNotMin(50)
	} else if amount < u.Credits/10 {
		return "", ErrGambleNotMin(u.Credits / 10)
	} else if amount > u.Credits {
		return "", ErrGambleNotEnough
	}
	twealth = u.Credits - amount
	spoils = gambleAlgorithm(59, 39, 2, amount)

	var msg string
	if spoils <= 0 {
		msg = fmt.Sprintf("%s gambled **%d** %s\n"+
			"Result: **loss**\n"+
			"%s remaining in bank: **%d**.",
			u.StringPretty(), amount, GambleCredits, strings.Title(GambleCredits), twealth)

		bu := UserNew(Bot.User)
		bu.Get(bu.ID)
		bu.Credits += amount
		err = bu.Update()
		if err != nil {
			return "", err
		}
	} else {
		twealth += spoils
		msg = fmt.Sprintf("%s gambled **%d** %s\n"+
			"Result: **Won**    spoils: **%d**\n"+
			"%s remaining in bank: **%d**.",
			u.StringPretty(), amount, GambleCredits, spoils, strings.Title(GambleCredits), twealth)
	}

	// twealth has new player bank amount.
	// Need to get difference and increment.
	u.Credits = twealth

	err = u.Update()
	if err != nil {
		return "", err
	}

	return msg, nil
}

// Transfer sends credits to another user.
func (u *User) Transfer(amount int, uID string) (string, error) {

	if amount < 0 {
		msg := "The authorities have been alerted with your attempt of theft!"
		return msg, nil
	} else if amount == 0 || uID == u.ID {
		msg := "What do you plan on accomplishing with this? Amount was 0. Use `-n Number`"
		return msg, nil
	}

	u2 := UserNew(nil)
	if err := u2.Get(uID); err != nil {
		return "", err
	}

	if u.Credits >= amount {
		u.Credits -= amount
		if err := u.Update(); err != nil {
			return "", err
		}

		u2.Credits += amount
		if err := u2.Update(); err != nil {
			return "", err
		}

		msg := fmt.Sprintf("%s has transferred __**%d**__ %s to %s.",
			u.StringPretty(), amount, GambleCredits, u2.StringPretty())

		return msg, nil
	}
	msg := fmt.Sprintf("You can't afford %d %s, you only have %d.",
		amount, GambleCredits, u.Credits)

	return msg, nil
}

// gambleAlgorithm for getting win/loss amount.
func gambleAlgorithm(l, d, t, credits int) int {
	s1 := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(s1)

	total := l + d + t
	if total != 100 {
		l = 60
		d = 38
		t = 2
	}

	num := r1.Intn(100)
	if num < l {
		// Lose all
		credits = 0
	} else if num >= l && num < l+d {
		// Win x2
		credits *= 2
	} else if num >= l+d {
		// Win x3
		credits *= 3
	}

	return credits
}

/*
	PERMISSION RELATED
	ACTIONS
*/

// Permission handles Adding and Removing permissions for a user.
func (u *User) Permission(fl userFlags) (string, error) {

	if ok := u.HasPermission(fl.server, permAdmin); !ok {
		return "", ErrBadPermissions
	} else if fl.Help {
		// Print Help + Syntax
		pre := "Requires an Action and an @User\n\n"
		return Help(fl.flag, pre, permSyntaxAll), nil
	} else if fl.List {
		// List users with non-default permissions.
		return "Listing hasn't been added yet.", nil

	}

	var msg string
	if (fl.Add || fl.Remove) && fl.Type != "" {
		if fl.User == "" {
			// Add permission Syntax help
			return permSyntaxAdd, nil
		}
		ids := strings.FieldsFunc(fl.User, idSplit)
		id := ids[0]
		// Get Permission Number
		var perm = permInt(fl.Type)
		if perm == 0 {
			return "Bad permission\n" + permList(), nil
		}
		// Add permission
		user := UserNew(nil)
		if err := user.Get(id); err != nil {
			return "", err
		}
		if fl.Add {
			user.PermissionAdd(fl.server, perm)
			if err := user.Update(); err != nil {
				return "", err
			}
			msg = fmt.Sprintf("%s has added the __**%s**__ permission to %s", u.StringPretty(), fl.Type, user.StringPretty())
		} else {
			user.PermissionDelete(fl.server, perm)
			if err := user.Update(); err != nil {
				return "", err
			}
			msg = fmt.Sprintf("%s has removed the __**%s**__ permission from %s", u.StringPretty(), fl.Type, user.StringPretty())
		}
	} else {
		pre := "Requires an Action and an @User\n\n"
		// Print Help here.
		return Help(fl.flag, pre, permSyntaxAll), nil
	}
	// Return result/text and nil error.
	return msg, nil
}

// HasPermission returns True if have permissions for action, false if not.
func (u *User) HasPermission(server string, access int) bool {
	var found bool
	var loc int
	for n, s := range u.Access {
		if s.ServerID == server {
			found = true
			loc = n
		}
	}

	if found {
		return u.Access[loc].Permissions&access == access
	}

	u.Access = append(u.Access, Access{ServerID: server, Permissions: permNormal})
	return false
}

// HasPermissionGTE to what is suppled. (Greater Than or Equal)
func (u *User) HasPermissionGTE(server string, access int) bool {
	var found bool
	var loc int
	for n, s := range u.Access {
		if s.ServerID == server {
			found = true
			loc = n
		}
	}

	if found {
		return u.Access[loc].Permissions >= access
	}

	return false
}

// PermissionAdd upgrades a user to new permissions
func (u *User) PermissionAdd(server string, access int) {
	for n, s := range u.Access {
		if s.ServerID == server {
			u.Access[n].Permissions |= access
			return
		}
	}

	u.Access = append(u.Access, Access{ServerID: server, ServerName: "", Permissions: permNormal | access})
	return
}

// PermissionDelete strips a permission from an User
func (u *User) PermissionDelete(server string, access int) {
	for n, s := range u.Access {
		if s.ServerID == server {
			u.Access[n].Permissions ^= access
			return
		}
	}

	newPerm := Access{ServerID: server, ServerName: "", Permissions: permNormal}
	newPerm.Permissions ^= access
	u.Access = append(u.Access, newPerm)

	return
}

// PermissionSet assigns a specific access level.
func (u *User) PermissionSet(server string, access int) {
	for n, s := range u.Access {
		if s.ServerID == server {
			u.Access[n].Permissions = access
			return
		}
	}

	newPerm := Access{ServerID: server, ServerName: "", Permissions: access}
	u.Access = append(u.Access, newPerm)

	return
}

// Convert a Permission from a String to an Int.
func permInt(p string) int {
	switch strings.ToLower(p) {
	case "admin":
		return permAdmin
	case "mod", "moderator":
		return permModerator
	default:
		return 0
	}
}

// Convert a Permission from an Int to a String.
func permString(p int) string {
	var permissions string
	for p > 0 {
		switch {
		case p&permAscended == permAscended:
			p ^= permAscended
			permissions += "Ascended "
		case p&permAdmin == permAdmin:
			p ^= permAdmin
			permissions += "Admin "
		case p&permModerator == permModerator:
			p ^= permModerator
			permissions += "Moderator "
		case p&permNormal == permNormal:
			p ^= permNormal
			permissions += "Base "
		default:
			// Unknown permission, break infinite loop.
			break
		}
	}
	perm := strings.Fields(permissions)
	return strings.Join(perm, ", ")
}

// Prints Permissions that can be modified.
func permList() string {
	return "Admin -> Ban permissions, special commands\nMod/Moderator -> Add Events\n"
}

/*
	Miscellanous Functions
*/

// Attempts to return an ID of a user despite <@ID> or Name#Discrim format.
func userIDClean(str string) string {
	if strings.ContainsRune(str, '@') {
		ids := strings.FieldsFunc(str, idSplit)
		id := ids[0]
		return id
	}
	return ""
}
