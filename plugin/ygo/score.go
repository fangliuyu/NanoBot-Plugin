// Package ygo 签到系统
package ygo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"github.com/sirupsen/logrus"

	"github.com/FloatTech/NanoBot-Plugin/utils/ctxext"
	ctrl "github.com/FloatTech/zbpctrl"
	nano "github.com/fumiama/NanoBot"

	// 数据库
	"github.com/FloatTech/AnimeAPI/wallet"
	fcext "github.com/FloatTech/floatbox/ctxext"
	sql "github.com/FloatTech/sqlite"

	// 图片输出
	"github.com/FloatTech/floatbox/file"
	"github.com/FloatTech/floatbox/web"
	"github.com/FloatTech/gg"
	"github.com/FloatTech/imgfactory"
	"github.com/FloatTech/zbputils/img/text"
)

type score struct {
	db *sql.Sqlite
	sync.RWMutex
}

// 用户数据信息
type userdata struct {
	Uid        uint64 // `Userid`
	UserName   string // `User`
	UpdatedAt  int64  // `签到时间`
	Continuous int    // `连续签到次数`
	Level      int    // `决斗者等级`
	UserPic    string // `头像图片`
	Picname    string // `签到图片`
}

const scoreMax = 1200

var (
	mu        sync.RWMutex
	scoredata = &score{
		db: &sql.Sqlite{},
	}
	/************************************10*****20******30*****40*****50******60*****70*****80******90**************/
	/*************************2******10*****20******40*****70*****110******160******220***290*****370*******460***************/
	levelrank = [...]string{"新手", "青铜", "白银", "黄金", "白金Ⅲ", "白金Ⅱ", "白金Ⅰ", "传奇Ⅲ", "传奇Ⅱ", "传奇Ⅰ", "决斗王"}
	engine    = nano.Register("score", &ctrl.Options[*nano.Ctx]{
		DisableOnDefault:  false,
		Brief:             "签到",
		PrivateDataFolder: "score",
		Help:              "- 签到\n- 获得签到背景",
	})
	cachePath = engine.DataFolder() + "cache/"
)

func init() {
	go func() {
		err := os.MkdirAll(cachePath, 0755)
		if err != nil {
			panic(err)
		}
		err = os.MkdirAll(cachePath+"other/", 0755)
		if err != nil {
			panic(err)
		}
	}()
	getdb := fcext.DoOnceOnSuccess(func(ctx *nano.Ctx) bool {
		scoredata.db.DBPath = engine.DataFolder() + "score.db"
		err := scoredata.db.Open(time.Hour * 24)
		if err != nil {
			_, _ = ctx.SendPlainMessage(false, "[init ERROR]:", err)
			return false
		}
		err = scoredata.db.Create("score", &userdata{})
		if err != nil {
			_, _ = ctx.SendPlainMessage(false, "[ERROR]:", err)
			return false
		}
		return true
	})

	engine.OnMessageFullMatchGroup([]string{"签到", "打卡"}, getdb).SetBlock(true).Handle(func(ctx *nano.Ctx) {
		uidinfo := ctx.Message.Author.ID
		if uidinfo == "" {
			_, _ = ctx.SendPlainMessage(false, "ERROR: 未获取到用户uid")
			return
		}
		uid, _ := strconv.ParseUint(uidinfo, 10, 64)

		userinfo := scoredata.getData(uid)
		userinfo.Uid = uid
		uInfo, err := ctx.GetGuildMemberOf(ctx.Message.GuildID, uidinfo)
		if err != nil {
			return
		}
		nick := uInfo.Nick
		if nick == "" {
			nick = uInfo.User.Username
		}
		userinfo.UserName = strings.TrimSpace(nick)
		userinfo.UserPic = uInfo.User.Avatar

		lasttime := time.Unix(userinfo.UpdatedAt, 0)
		score := wallet.GetWalletOf(int64(uid))
		// 判断是否已经签到过了
		if time.Now().Format("2006/01/02") == lasttime.Format("2006/01/02") {
			if userinfo.Picname == "" {
				picFile, err := initPic()
				if err != nil {
					_, _ = ctx.SendPlainMessage(false, "[ERROR]:", err)
					return
				}
				if picFile != "" {
					userinfo.Picname = picFile
					if err := scoredata.setData(userinfo); err != nil {
						ctx.SendPlainMessage(true, "签到记录失败。\n", err)
						return
					}
				}
			}
			data, err := drawimagePro(&userinfo, score, 0)
			if err != nil {
				_, _ = ctx.SendPlainMessage(false, "[ERROR]:", err)
				return
			}
			ctx.SendChain(nano.ReplyTo(ctx.Message.ID), nano.Text("今天已经签到过了"), nano.ImageBytes(data))
			return
		}
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			picFile, err := initPic()
			if err != nil {
				_, _ = ctx.SendPlainMessage(false, "[ERROR]:", err)
				return
			}
			if picFile == "" {
				return
			}
			userinfo.Picname = picFile
			if err := scoredata.setData(userinfo); err != nil {
				ctx.SendPlainMessage(true, "签到记录失败。\n", err)
				return
			}
		}()
		add := rand.Intn(10 + userinfo.Level/2)
		wg.Add(1)
		go func() {
			// 更新数据
			subtime := time.Since(lasttime).Hours()
			if subtime > 48 {
				userinfo.Continuous = 1
			} else {
				userinfo.Continuous += 1
				add = int(math.Min(5, float64(userinfo.Continuous)))
			}
			userinfo.UpdatedAt = time.Now().Unix()
			if userinfo.Level < scoreMax {
				userinfo.Level += add
			}
			defer wg.Done()
			if err := scoredata.setData(userinfo); err != nil {
				ctx.SendPlainMessage(true, "签到记录失败。\n", err)
				return
			}
			level, _ := getLevel(userinfo.Level)
			if err := wallet.InsertWalletOf(int64(uid), add+level*5); err != nil {
				ctx.SendPlainMessage(true, "货币记录失败。\n", err)
				return
			}
			score = wallet.GetWalletOf(int64(uid))
		}()
		// 生成签到图片
		wg.Wait()
		data, err := drawimagePro(&userinfo, score, add)
		if err != nil {
			_, _ = ctx.SendPlainMessage(false, "[ERROR]:", err)
			return
		}
		ctx.SendImageBytes(data, true)
	})
	engine.OnMessageFullMatch("获得签到背景").Limit(ctxext.LimitByGroup).SetBlock(true).
		Handle(func(ctx *nano.Ctx) {
			uidinfo := ctx.Message.Author.ID
			if uidinfo == "" {
				_, _ = ctx.SendPlainMessage(false, "ERROR: 未获取到用户uid")
				return
			}
			uid, _ := strconv.ParseUint(uidinfo, 10, 64)
			userinfo := scoredata.getData(uid)
			picFile := userinfo.Picname
			if file.IsNotExist(picFile) {
				ctx.SendPlainMessage(true, "请先签到！")
				return
			}
			ctx.SendImage("file:///"+file.BOTPATH+"/"+picFile, true)
		})
}

// 获取签到数据
func (sdb *score) getData(uid uint64) (userinfo userdata) {
	sdb.Lock()
	defer sdb.Unlock()
	_ = sdb.db.Find("score", &userinfo, "where uid = "+strconv.FormatUint(uid, 10))
	return
}

// 保存签到数据
func (sdb *score) setData(userinfo userdata) error {
	sdb.Lock()
	defer sdb.Unlock()
	return sdb.db.Insert("score", &userinfo)

}

type datajson struct {
	Code string   `json:"code"`
	Pic  []string `json:"pic"`
}

// 下载图片
func initPic() (picFile string, err error) {
	// defer process.SleepAbout1sTo2s()
	data, err := web.GetData("https://img.moehu.org/pic.php?return=json&id=yu-gi-oh&num=1")
	if err != nil { // 如果api跑路了抽本地
		logrus.Warnln("[score] 访问api失败,将从本地抽取:", err)
		return randFile(3)
	}
	parsed := datajson{}
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		logrus.Warnln("[score] 解析api失败,将从本地抽取:", err)
		return randFile(3)
	}
	if len(parsed.Pic) == 0 {
		return "", errors.New("no picData")
	}
	names := strings.Split(parsed.Pic[0], "/")
	picFile = cachePath + names[len(names)-1]
	if file.IsExist(picFile) {
		return picFile, nil
	}
	mu.Lock()
	defer mu.Unlock()
	err = file.DownloadTo(parsed.Pic[0], picFile)
	if err != nil {
		logrus.Warnln("[score] 下载图片失败,将从下载其他二次元图片:", err)
		return otherPic()
	}
	return picFile, nil
}

// 下载图片
func otherPic() (picFile string, err error) {
	apiList := []string{"http://81.70.100.130/api/DmImgS.php", "http://81.70.100.130/api/DmImg.php", "http://81.70.100.130/api/acgimg.php"}
	picFile = "other/" + time.Now().Format("20060102150405000") + ".jpeg"
	mu.Lock()
	defer mu.Unlock()
	err = file.DownloadTo(apiList[rand.Intn(len(apiList))], picFile)
	if err != nil {
		logrus.Warnln("[score] 下载图片失败,将从本地抽取:", err)
		return randFile(3)
	}
	return picFile, nil
}

func randFile(indexMax int) (string, error) {
	files, err := os.ReadDir(file.BOTPATH + cachePath)
	if err != nil {
		return "", err
	}
	if len(files) > 0 {
		drawFile := files[rand.Intn(len(files))].Name()
		// 如果是文件夹就递归
		before, _, ok := strings.Cut(drawFile, ".")
		if !ok || before == "" {
			indexMax--
			if indexMax <= 0 {
				return "", errors.New("存在太多非图片文件,请清理~")
			}
			return randFile(indexMax)
		}
		return cachePath + drawFile, err
	}
	return "", errors.New("不存在本地签到图片")
}

func drawimagePro(userinfo *userdata, score, add int) (data []byte, err error) {
	if userinfo.Picname == "" {
		err = errors.New("[ERROR]:签到图片获取失败")
		return
	}
	back, err := gg.LoadImage(userinfo.Picname)
	if err != nil {
		return
	}
	imgDX := back.Bounds().Dx()
	imgDY := back.Bounds().Dy()
	backDX := 1500

	imgDW := backDX - 100
	scale := float64(imgDW) / float64(imgDX)
	imgDH := int(float64(imgDY) * scale)
	back = imgfactory.Size(back, imgDW, imgDH).Image()

	backDY := imgDH + 500
	canvas := gg.NewContext(backDX, backDY)
	// 放置毛玻璃背景
	backBlurW := float64(imgDW) * (float64(backDY) / float64(imgDH))
	canvas.DrawImageAnchored(imaging.Blur(imgfactory.Size(back, int(backBlurW), backDY).Image(), 8), backDX/2, backDY/2, 0.5, 0.5)
	canvas.DrawRectangle(1, 1, float64(backDX), float64(backDY))
	canvas.SetLineWidth(3)
	canvas.SetRGBA255(255, 255, 255, 100)
	canvas.StrokePreserve()
	canvas.SetRGBA255(255, 255, 255, 140)
	canvas.Fill()
	// 信息框
	canvas.DrawRoundedRectangle(20, 20, 1500-20-20, 450-20, (450-20)/5)
	canvas.SetLineWidth(6)
	canvas.SetDash(20.0, 10.0, 0)
	canvas.SetRGBA255(255, 255, 255, 255)
	canvas.Stroke()
	// 放置头像
	getAvatar, err := web.GetData(userinfo.UserPic)
	if err != nil {
		return
	}
	avatar, _, err := image.Decode(bytes.NewReader(getAvatar))
	if err != nil {
		return
	}
	avatarf := imgfactory.Size(avatar, 270, 270)
	canvas.DrawCircle(50+float64(avatarf.W())/2, 50+float64(avatarf.H())/2, float64(avatarf.W())/2+2)
	canvas.SetLineWidth(3)
	canvas.SetDash()
	canvas.SetRGBA255(255, 255, 255, 255)
	canvas.Stroke()
	canvas.DrawImage(avatarf.Circle(0).Image(), 50, 50)
	// 放置昵称
	canvas.SetRGB(0, 0, 0)
	fontSize := 150.0
	fontdata, err := file.GetLazyData(text.BoldFontFile, nano.Md5File, false)
	if err != nil {
		return
	}
	if err = canvas.ParseFontFace(fontdata, fontSize); err != nil {
		return
	}
	nameW, nameH := canvas.MeasureString(userinfo.UserName)
	// 昵称范围
	textH := 300.0
	textW := float64(backDX) * 2 / 3
	// 如果文字超过长度了，比列缩小字体
	if nameW > textW {
		scale := 2 * nameH / textH
		fontSize = fontSize * scale
		if err = canvas.ParseFontFace(fontdata, fontSize); err != nil {
			return
		}
		_, nameH := canvas.MeasureString(userinfo.UserName)
		// 昵称分段
		name := []rune(userinfo.UserName)
		names := make([]string, 0, 4)
		// 如果一半都没到界面边界就分两行
		wordw, _ := canvas.MeasureString(string(name[:len(name)/2]))
		if wordw < textW*3/4 {
			names = append(names, string(name[:len(name)/2+1]))
			names = append(names, string(name[len(name)/2+1:]))
		} else {
			nameLength := 0.0
			lastIndex := 0
			for i, word := range name {
				wordw, _ = canvas.MeasureString(string(word))
				nameLength += wordw
				if nameLength > textW*3/4 || i == len(name)-1 {
					names = append(names, string(name[lastIndex:i+1]))
					lastIndex = i + 1
					nameLength = 0
				}
			}
			// 超过两行就重新配置一下字体大小
			scale = float64(len(names)) * nameH / textH
			fontSize = fontSize * scale
			if err = canvas.ParseFontFace(fontdata, fontSize); err != nil {
				return
			}
		}
		for i, nameSplit := range names {
			canvas.DrawStringAnchored(nameSplit, float64(backDX)/2+25, 25+(200+70*scale)*float64(i+1)/float64(len(names))-nameH/2, 0.5, 0.5)
		}
	} else {
		canvas.DrawStringAnchored(userinfo.UserName, float64(backDX)/2+25, 200-nameH/2, 0.5, 0.5)
	}

	// level
	if err = canvas.ParseFontFace(fontdata, 72); err != nil {
		return
	}
	level, nextLevelScore := getLevel(userinfo.Level)
	if level == -1 {
		err = errors.New("计算等级出现了问题")
		return
	}
	rank := levelrank[level]
	textW, textH = canvas.MeasureString(rank)
	levelX := float64(backDX) * 4 / 5
	canvas.DrawRoundedRectangle(levelX, 50, textW*1.2, 200, 200/5)
	canvas.SetLineWidth(3)
	canvas.SetRGBA255(0, 0, 0, 100)
	canvas.StrokePreserve()
	canvas.SetRGBA255(255, 255, 255, 100)
	canvas.Fill()
	canvas.DrawRoundedRectangle(levelX, 50, textW*1.2, 100, 200/5)
	canvas.SetLineWidth(3)
	canvas.SetRGBA255(0, 0, 0, 100)
	canvas.StrokePreserve()
	canvas.SetRGBA255(255, 255, 255, 100)
	canvas.Fill()
	canvas.SetRGBA255(0, 0, 0, 255)
	canvas.DrawStringAnchored(levelrank[level], levelX+textW*1.2/2, 50+50, 0.5, 0.5)
	canvas.DrawStringAnchored(fmt.Sprintf("LV%d", level), levelX+textW*1.2/2, 50+100+50, 0.5, 0.5)

	if add == 0 {
		canvas.DrawStringAnchored(fmt.Sprintf("已连签 %d 天    总资产: %d", userinfo.Continuous, score), float64(backDX)/2, 370-textH/2, 0.5, 0.5)
	} else {
		canvas.DrawStringAnchored(fmt.Sprintf("连签 %d 天 总资产(+%d): %d", userinfo.Continuous, add+level*5, score), float64(backDX)/2, 370-textH/2, 0.5, 0.5)
	}
	// 绘制等级进度条
	if err = canvas.ParseFontFace(fontdata, 50); err != nil {
		return
	}
	_, textH = canvas.MeasureString("/")
	switch {
	case userinfo.Level < scoreMax && add == 0:
		canvas.DrawStringAnchored(fmt.Sprintf("%d/%d", userinfo.Level, nextLevelScore), float64(backDX)/2, 455-textH, 0.5, 0.5)
	case userinfo.Level < scoreMax:
		canvas.DrawStringAnchored(fmt.Sprintf("(%d+%d)/%d", userinfo.Level-add, add, nextLevelScore), float64(backDX)/2, 455-textH, 0.5, 0.5)
	default:
		canvas.DrawStringAnchored("Max/Max", float64(backDX)/2, 455-textH, 0.5, 0.5)

	}
	// 创建彩虹条
	grad := gg.NewLinearGradient(0, 500, 1500, 300)
	grad.AddColorStop(0, color.RGBA{G: 255, A: 255})
	grad.AddColorStop(0.35, color.RGBA{B: 255, A: 255})
	grad.AddColorStop(0.5, color.RGBA{R: 255, A: 255})
	grad.AddColorStop(0.65, color.RGBA{B: 255, A: 255})
	grad.AddColorStop(1, color.RGBA{G: 255, A: 255})
	canvas.SetStrokeStyle(grad)
	canvas.SetLineWidth(7)
	// 设置长度
	gradMax := 1300.0
	LevelLength := gradMax * (float64(userinfo.Level) / float64(nextLevelScore))
	canvas.MoveTo((float64(backDX)-LevelLength)/2, 450)
	canvas.LineTo((float64(backDX)+LevelLength)/2, 450)
	canvas.ClosePath()
	canvas.Stroke()
	// 放置图片
	canvas.DrawImageAnchored(back, backDX/2, imgDH/2+475, 0.5, 0.5)
	// 生成图片
	return imgfactory.ToBytes(canvas.Image())
}
func getLevel(count int) (int, int) {
	switch {
	case count < 5:
		return 0, 5
	case count > scoreMax:
		return len(levelrank) - 1, scoreMax
	default:
		for k, i := 1, 10; i <= scoreMax; i += (k * 10) * scoreMax / 460 {
			if count < i {
				return k, i
			}
			k++
		}
	}
	return -1, -1
}
