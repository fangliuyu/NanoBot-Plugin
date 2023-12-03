// Package ygo 一些关于ygo的插件
package ygo

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"encoding/json"

	"github.com/FloatTech/floatbox/web"
	ctrl "github.com/FloatTech/zbpctrl"
	nano "github.com/fumiama/NanoBot"
)

const (
	api     = "https://ygocdb.com/api/v0/?search="
	picherf = "https://cdn.233.momobako.com/ygopro/pics/"
)

type searchResult struct {
	Result []struct {
		Cid    int    `json:"cid"`
		ID     int    `json:"id"`
		CnName string `json:"cn_name"`
		// CnocgN string `json:"cnocg_n"` // 简中卡名
		JpName string `json:"jp_name"`
		EnName string `json:"en_name"`
		Text   struct {
			Types string `json:"types"`
			Pdesc string `json:"pdesc"`
			Desc  string `json:"desc"`
		} `json:"text"`
	} `json:"result"`
}

func init() {
	en := nano.Register("ygocdb", &ctrl.Options[*nano.Ctx]{
		DisableOnDefault: false,
		Brief:            "游戏王百鸽API", // 本插件基于游戏王百鸽API"https://www.ygo-sem.cn/"
		Help: "- /yc [xxx]\n" +
			"- /ycp [xxx]\n" +
			"- /ys [xxx]\n" +
			"[xxx]为搜索内容",
	})

	en.OnMessageRegex(`^\/yc([^p].*)`).SetBlock(true).Handle(func(ctx *nano.Ctx) {
		ctxtext := ctx.State["regex_matched"].([]string)[1]
		if ctxtext == "" {
			ctx.SendPlainMessage(true, "你是想查询「空手假象」吗？")
			return
		}
		ctxtext = strings.TrimSpace(ctxtext)
		data, err := web.GetData(api + url.QueryEscape(ctxtext))
		if err != nil {
			ctx.SendPlainMessage(false, "[ERROR]:", err)
			return
		}
		var result searchResult
		err = json.Unmarshal(data, &result)
		if err != nil {
			ctx.SendPlainMessage(false, "[ERROR]:", err)
			return
		}
		maxpage := len(result.Result)
		if maxpage == 0 {
			ctx.SendPlainMessage(true, "没有找到相关的卡片额")
			return
		}
		cardtextout := cardtext(result, 0)
		ctx.SendPlainMessage(true, cardtextout)
	})

	en.OnMessageRegex(`^\/ycp(.*)`).SetBlock(true).Handle(func(ctx *nano.Ctx) {
		ctxtext := ctx.State["regex_matched"].([]string)[1]
		if ctxtext == "" {
			ctx.SendPlainMessage(true, "你是想查询「空手假象」吗？")
			return
		}
		ctxtext = strings.TrimSpace(ctxtext)
		data, err := web.GetData(api + url.QueryEscape(ctxtext))
		if err != nil {
			ctx.SendPlainMessage(false, "[ERROR]:", err)
			return
		}
		var result searchResult
		err = json.Unmarshal(data, &result)
		if err != nil {
			ctx.SendPlainMessage(false, "[ERROR]:", err)
			return
		}
		maxpage := len(result.Result)
		if maxpage == 0 {
			ctx.SendPlainMessage(true, "没有找到相关的卡片额")
			return
		}
		ctx.SendImage(picherf+strconv.Itoa(result.Result[0].ID)+".jpg", true)
	})

	en.OnMessageRegex(`^\/ys(.*)`).SetBlock(true).Handle(func(ctx *nano.Ctx) {
		ctxtext := ctx.State["regex_matched"].([]string)[1]
		if ctxtext == "" {
			ctx.SendPlainMessage(true, "你是想查询「空手假象」吗？")
			return
		}
		ctxtext = strings.TrimSpace(ctxtext)
		data, err := web.GetData(api + url.QueryEscape(ctxtext))
		if err != nil {
			ctx.SendPlainMessage(false, "[ERROR]:", err)
			return
		}
		var result searchResult
		err = json.Unmarshal(data, &result)
		if err != nil {
			ctx.SendPlainMessage(false, "[ERROR]:", err)
			return
		}
		maxpage := len(result.Result)
		if maxpage == 0 {
			ctx.SendPlainMessage(true, "没有找到相关的卡片额")
			return
		}
		if maxpage == 1 {
			cardtextout := cardtext(result, 0)
			ctx.SendChain(nano.ReplyTo(ctx.Message.ID), nano.Image(picherf+strconv.Itoa(result.Result[0].ID)+".jpg"), nano.Text(cardtextout))
			return
		}
		var listName []string
		var listid []int
		for _, v := range result.Result {
			listName = append(listName, strconv.Itoa(len(listName))+"."+v.CnName)
			listid = append(listid, v.ID)
		}
		var (
			currentPage = 10
			nextpage    = 0
		)
		if maxpage < 10 {
			currentPage = maxpage
		}
		ctx.SendChain(nano.ReplyTo(ctx.Message.ID), nano.Text("找到", strconv.Itoa(maxpage), "张相关卡片,当前显示以下卡名：\n",
			strings.Join(listName[:currentPage], "\n"),
			"\n————————————\n输入对应数字获取卡片信息,",
			"\n或回复“取消”、“下一页”指令"))
		var next *nano.FutureEvent = nano.NewFutureEvent("Message", 999, false, nano.RegexRule(`^(取消|下一页|\d+)$`),
			ctx.CheckSession())
		recv, cancel := next.Repeat()
		defer cancel()
		after := time.NewTimer(60 * time.Second)
		for {
			select {
			case <-after.C:
				ctx.SendPlainMessage(true, "等待超时,搜索结束")
				return
			case e := <-recv:
				nextcmd := e.Message.Content
				switch nextcmd {
				case "取消":
					after.Stop()
					e.SendPlainMessage(true, "用户取消,搜索结束")
					return
				case "下一页":
					after.Reset(60 * time.Second)
					if maxpage < 11 {
						continue
					}
					nextpage++
					if nextpage*10 >= maxpage {
						nextpage = 0
						currentPage = 10
						e.SendPlainMessage(true, "已是最后一页，返回到第一页")
					} else if nextpage == maxpage/10 {
						currentPage = maxpage % 10
					}
					e.SendChain(nano.ReplyTo(e.Message.ID), nano.Text("找到", strconv.Itoa(maxpage), "张相关卡片,当前显示以下卡名：\n",
						strings.Join(listName[nextpage*10:nextpage*10+currentPage], "\n"),
						"\n————————————————\n输入对应数字获取卡片信息,",
						"\n或回复“取消”、“下一页”指令"))
				default:
					cardint, err := strconv.Atoi(nextcmd)
					switch {
					case err != nil:
						after.Reset(60 * time.Second)
						e.SendPlainMessage(true, "请输入正确的序号")
					default:
						if cardint < nextpage*10+currentPage {
							after.Stop()
							cardtextout := cardtext(result, cardint)
							ctx.SendChain(nano.ReplyTo(ctx.Message.ID), nano.Image(picherf+strconv.Itoa(listid[cardint])+".jpg"), nano.Text(cardtextout))
							return
						}
						after.Reset(60 * time.Second)
						e.SendPlainMessage(true, "请输入正确的序号")
					}
				}
			}
		}
	})
}

func cardtext(list searchResult, cardid int) string {
	var cardtext []string
	cardtext = append(cardtext, "中文卡名：\n    "+list.Result[cardid].CnName)
	if list.Result[cardid].JpName == "" {
		cardtext = append(cardtext, "英文卡名：\n    "+list.Result[cardid].EnName)
	} else {
		cardtext = append(cardtext, "日文卡名：\n    "+list.Result[cardid].JpName)
	}
	cardtext = append(cardtext, "卡片密码："+strconv.Itoa(list.Result[cardid].ID))
	cardtext = append(cardtext, list.Result[cardid].Text.Types)
	if list.Result[cardid].Text.Pdesc != "" {
		cardtext = append(cardtext, "[灵摆效果]\n"+list.Result[cardid].Text.Pdesc)
		if strings.Contains(list.Result[cardid].Text.Types, "效果") {
			cardtext = append(cardtext, "[怪兽效果]")
		} else {
			cardtext = append(cardtext, "[怪兽描述]")
		}
	}
	cardtext = append(cardtext, list.Result[cardid].Text.Desc)
	return strings.Join(cardtext, "\n")
}
