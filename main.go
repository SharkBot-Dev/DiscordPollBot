package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SharkBot-Dev/DiscordPollBot/lib"
	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

type Poll struct {
	Title      string
	Options    []string
	Votes      map[string][]string
	MaxChoices int
	CreatorID  string
}

var (
	session  *discordgo.Session
	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "help",
			Description: "Botの使い方を知ります",
		},
		{
			Name:        "create-poll",
			Description: "匿名投票を作成します（単一・複数選択対応）。",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "title",
					Description: "投票のタイトル",
					Required:    true,
					Type:        discordgo.ApplicationCommandOptionString,
				},
				{
					Name:        "option1",
					Description: "選択肢1を入力",
					Required:    true,
					Type:        discordgo.ApplicationCommandOptionString,
				},
				{
					Name:        "option2",
					Description: "選択肢2を入力",
					Required:    true,
					Type:        discordgo.ApplicationCommandOptionString,
				},
				{
					Name:        "max-choices",
					Description: "最大何個まで選択できるか（デフォルト: 1）",
					Required:    false,
					Type:        discordgo.ApplicationCommandOptionInteger,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "1つだけ (単一選択)", Value: 1},
						{Name: "最大 2 個まで", Value: 2},
						{Name: "最大 3 個まで", Value: 3},
						{Name: "最大 4 個まで", Value: 4},
						{Name: "最大 5 個まで", Value: 5},
					},
				},
				{
					Name:        "option3",
					Description: "選択肢3を入力",
					Required:    false,
					Type:        discordgo.ApplicationCommandOptionString,
				},
				{
					Name:        "option4",
					Description: "選択肢4を入力",
					Required:    false,
					Type:        discordgo.ApplicationCommandOptionString,
				},
				{
					Name:        "option5",
					Description: "選択肢5を入力",
					Required:    false,
					Type:        discordgo.ApplicationCommandOptionString,
				},
			},
		},
	}
	startTime time.Time

	polls = make(map[string]*Poll)
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file, mapping to env variables directly")
	}

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("環境変数 DISCORD_TOKEN が設定されていません")
	}

	sessionManager := &lib.DiscordSessionManager{}
	session = sessionManager.InitializeSession(token)

	session.Identify.Intents = discordgo.IntentsGuilds

	if saveData, err := os.Open("saves/polls.json"); err == nil {
		saveJsonData, _ := io.ReadAll(saveData)
		json.Unmarshal(saveJsonData, &polls)
		saveData.Close()
		log.Print("セーブデータをロードしました。")
	}

	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Print("起動しました。")
		go func() {
			_, err := s.ApplicationCommandBulkOverwrite(s.State.Application.ID, "", commands)
			if err != nil {
				log.Printf("コマンド同期エラー: %v", err)
			}
			log.Print("スラッシュコマンドを同期しました。")
		}()
	})

	session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			commandName := i.ApplicationCommandData().Name
			switch commandName {
			case "help":
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Embeds: []*discordgo.MessageEmbed{
							{
								Title: "匿名投票Botの使い方",
								Color: 4712039,
								Fields: []*discordgo.MessageEmbedField{
									{
										Name:  "匿名投票の作り方",
										Value: "/create-poll でタイトルと選択肢を入力して作成します。\n`max-choices` を指定すれば複数投票も可能です。",
									},
									{
										Name:  "匿名投票の終了方法",
										Value: "「投票を終了する」ボタンで投票を終了できます。",
									},
								},
							},
						},
						Flags: discordgo.MessageFlagsEphemeral,
					},
				})

			case "create-poll":
				optionsMap := make(map[string]interface{})
				for _, opt := range i.ApplicationCommandData().Options {
					if opt.Type == discordgo.ApplicationCommandOptionInteger {
						optionsMap[opt.Name] = opt.IntValue()
					} else {
						optionsMap[opt.Name] = opt.StringValue()
					}
				}

				title, _ := optionsMap["title"].(string)

				maxChoices := 1
				if val, ok := optionsMap["max-choices"].(int64); ok {
					maxChoices = int(val)
				}

				var options []string
				for idx := 1; idx <= 5; idx++ {
					if optVal, ok := optionsMap[fmt.Sprintf("option%d", idx)].(string); ok && optVal != "" {
						options = append(options, optVal)
					}
				}

				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Flags: discordgo.MessageFlagsEphemeral,
					},
				})

				dummyVotes := make(map[string][]string)
				embed := renderPollEmbed(title, options, dummyVotes, maxChoices, false)
				components := renderPollComponents(options, false)

				pollMessage, err := s.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{
					Embeds:     []*discordgo.MessageEmbed{embed},
					Components: components,
				})
				if err != nil {
					log.Printf("投票メッセージ送信エラー: %v", err)
					return
				}

				polls[pollMessage.ID] = &Poll{
					Title:      title,
					Options:    options,
					Votes:      make(map[string][]string),
					MaxChoices: maxChoices,
					CreatorID:  i.Member.User.ID,
				}

				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
					Embeds: &[]*discordgo.MessageEmbed{
						{
							Title:       "投票を作成しました。",
							Description: "「投票を終了する」ボタンで投票を終了できます。",
							Color:       4712039,
						},
					},
				})
			}

		case discordgo.InteractionMessageComponent:
			msgID := i.Message.ID
			poll, exists := polls[msgID]
			if !exists {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "エラー: この投票データは見つかりません（Botが再起動した可能性があります）。",
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
				return
			}

			customID := i.MessageComponentData().CustomID
			userID := i.Member.User.ID

			if customID == "end_poll" {
				if userID != poll.CreatorID {
					s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Content: "投票を終了できるのは、この投票の作成者だけです。",
							Flags:   discordgo.MessageFlagsEphemeral,
						},
					})
					return
				}

				finalEmbed := renderPollEmbed(poll.Title, poll.Options, poll.Votes, poll.MaxChoices, true)
				disabledComponents := renderPollComponents(poll.Options, true)

				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseUpdateMessage,
					Data: &discordgo.InteractionResponseData{
						Embeds:     []*discordgo.MessageEmbed{finalEmbed},
						Components: disabledComponents,
					},
				})

				counts := make(map[string]int)
				totalVotes := 0
				for _, userVotes := range poll.Votes {
					for _, optIdx := range userVotes {
						counts[optIdx]++
						totalVotes++
					}
				}

				maxVotes := 0
				var winnerIndices []int
				if totalVotes > 0 {
					for idx := range poll.Options {
						count := counts[fmt.Sprintf("%d", idx)]
						if count > maxVotes {
							maxVotes = count
							winnerIndices = []int{idx}
						} else if count == maxVotes && count > 0 {
							winnerIndices = append(winnerIndices, idx)
						}
					}
				}

				winnerText := "なし（投票がありませんでした）"
				if len(winnerIndices) > 0 {
					winnerText = ""
					for idx, wIdx := range winnerIndices {
						if idx > 0 {
							winnerText += ", "
						}
						winnerText += fmt.Sprintf("「%s」", poll.Options[wIdx])
					}
					winnerText += fmt.Sprintf(" (%d 票)", maxVotes)
				}

				_, replyErr := s.ChannelMessageSendEmbedReply(i.ChannelID, &discordgo.MessageEmbed{
					Title:       "📊 集計結果",
					Description: fmt.Sprintf("最終結果: 総得票数 %d 票\n🏆 **結果: %s**", totalVotes, winnerText),
					Color:       10066329,
				}, i.Message.Reference())

				if replyErr != nil {
					log.Print("集計エラー: ", replyErr)
				}

				delete(polls, msgID)
				return
			}

			currentVotes := poll.Votes[userID]
			foundIdx := -1
			for idx, v := range currentVotes {
				if v == customID {
					foundIdx = idx
					break
				}
			}

			if foundIdx != -1 {
				poll.Votes[userID] = append(currentVotes[:foundIdx], currentVotes[foundIdx+1:]...)
			} else {
				if poll.MaxChoices <= 0 {
					poll.MaxChoices = 1
				}

				if len(currentVotes) >= poll.MaxChoices {
					currentVotes = currentVotes[1:]
				}
				poll.Votes[userID] = append(currentVotes, customID)
			}

			updatedEmbed := renderPollEmbed(poll.Title, poll.Options, poll.Votes, poll.MaxChoices, false)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
				Data: &discordgo.InteractionResponseData{
					Embeds:     []*discordgo.MessageEmbed{updatedEmbed},
					Components: i.Message.Components,
				},
			})
		}
	})

	if err := session.Open(); err != nil {
		log.Fatalf("Discordセッションのオープンに失敗: %v", err)
	}

	startTime = time.Now()
	defer onClose()

	log.Println("ボットが起動しました。Ctrl+Cで終了します。")
	waitForExitSignal()
}

func onClose() {
	saveData, saveDataErr := json.Marshal(polls)
	if saveDataErr != nil {
		log.Fatalf("セーブデータ解析に失敗しました: %v", saveDataErr)
	}
	SaveErr := os.WriteFile("saves/polls.json", saveData, 0644)
	if SaveErr != nil {
		log.Fatalf("セーブに失敗しました: %v", SaveErr)
	}
	fmt.Println("セーブが完了しました。")

	session.Close()
}

func renderPollEmbed(title string, options []string, votes map[string][]string, maxChoices int, isEnded bool) *discordgo.MessageEmbed {
	counts := make(map[string]int)
	totalVotes := 0
	for _, userVotes := range votes {
		for _, optIdx := range userVotes {
			counts[optIdx]++
			totalVotes++
		}
	}

	var fields []*discordgo.MessageEmbedField
	for idx, opt := range options {
		count := counts[fmt.Sprintf("%d", idx)]
		percentage := 0.0
		if totalVotes > 0 {
			percentage = (float64(count) / float64(totalVotes)) * 100
		}

		barLength := 10
		filledLength := 0
		if totalVotes > 0 {
			filledLength = int((float64(count) / float64(totalVotes)) * float64(barLength))
		}
		bar := ""
		for i := 0; i < barLength; i++ {
			if i < filledLength {
				bar += "█"
			} else {
				bar += "░"
			}
		}

		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("%d. %s", idx+1, opt),
			Value:  fmt.Sprintf("%s %d 票 (%.1f%%)", bar, count, percentage),
			Inline: false,
		})
	}

	embedTitle := "📊 " + title

	typeText := "単一選択"
	if maxChoices > 1 {
		typeText = fmt.Sprintf("複数選択: 最大 %d 個まで", maxChoices)
	}

	description := fmt.Sprintf("投票形式: **%s**\n総得票数: %d 票", typeText, totalVotes)
	footerText := "ボタンを押して投票 / 同じボタンで投票取消"
	color := 4712039

	if isEnded {
		embedTitle = "🔒 [投票終了] " + title
		description = fmt.Sprintf("最終結果: 総得票数 %d 票\n*(この投票は締め切られました)*", totalVotes)
		footerText = "この投票は終了しました"
		color = 10066329
	}

	return &discordgo.MessageEmbed{
		Title:       embedTitle,
		Description: description,
		Color:       color,
		Fields:      fields,
		Footer: &discordgo.MessageEmbedFooter{
			Text: footerText,
		},
	}
}

func renderPollComponents(options []string, isEnded bool) []discordgo.MessageComponent {
	var voteButtons []discordgo.MessageComponent

	for idx, opt := range options {
		label := fmt.Sprintf("%d. %s", idx+1, opt)
		if len(label) > 50 {
			label = label[:47] + "..."
		}

		voteButtons = append(voteButtons, discordgo.Button{
			Label:    label,
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("%d", idx),
			Disabled: isEnded,
		})
	}

	endButton := discordgo.Button{
		Label:    "投票を終了する",
		Style:    discordgo.DangerButton,
		CustomID: "end_poll",
		Disabled: isEnded,
	}

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: voteButtons,
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{endButton},
		},
	}
}

func waitForExitSignal() {
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}
