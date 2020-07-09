package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cheggaaa/pb"
	"github.com/deepsheth/soup" // this fork is able to do HTTP Posts
	"github.com/fatih/color"
	"github.com/jinzhu/configor"
	"github.com/tidwall/gjson"
)

func download(url string) string {
	errored := false
	outputFolder := "Downloads"

retry:
	resp, err := http.Get(url)
	if err != nil {
		color.Red("### Unable to download\n\n")
		return ""
	}
	defer resp.Body.Close()

	_, params, _ := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	filePath := filepath.Join(outputFolder, params["filename"])

	if _, err := os.Stat(outputFolder); os.IsNotExist(err) {
		os.MkdirAll(outputFolder, 0700)
	}

	if !errored {
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			color.Red("### Exists\n\n")
			return filePath
		}
	}

	out, _ := os.Create(filePath)
	defer out.Close()

	i, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
	var sourceSize int64 = int64(i)

	bar := pb.New(int(sourceSize)).SetUnits(pb.U_BYTES).SetRefreshRate(time.Millisecond * 10)
	bar.ShowSpeed = true
	bar.Start()

	reader := bar.NewProxyReader(resp.Body)

	if _, err = io.Copy(out, reader); err != nil {
		color.Red("### Error Saving\n\n")
		errored = true
		bar.Finish()
		goto retry
	}

	bar.Finish()
	println()
	return filePath
}

func getPopplersFromSelectDownloadPage(selectDownloadURL string) string {
	parsedURL, _ := url.Parse(selectDownloadURL)
	urlQuerys := parsedURL.Query()

	if urlQuerys.Get("from") == "collection" {
		selectDownloadPageHTML, _ := soup.Get(selectDownloadURL)
		urlQuerys.Set("id", regexp.MustCompile(`id=(\d*)&`).FindStringSubmatch(selectDownloadPageHTML)[1])
		urlQuerys.Set("type", regexp.MustCompile(`\/download\/(album|track)\?`).FindStringSubmatch(selectDownloadPageHTML)[1])
	}

	params := url.Values{}
	params.Add("enc", "flac")
	params.Add("id", urlQuerys.Get("id"))
	params.Add("payment_id", urlQuerys.Get("payment_id"))
	params.Add("sig", urlQuerys.Get("sig"))
	params.Add(".rand", "1234567891234")
	params.Add(".vrs", "1")

	popplersPage, _ := soup.Get("https://popplers5.bandcamp.com/statdownload/" + urlQuerys.Get("type") + "?" + params.Encode())
	jsonString := regexp.MustCompile(`{\".*\"}`).FindString(popplersPage)
	downloadURL := gjson.Get(jsonString, "download_url").String()

	color.New(color.FgGreen).Print(string("==> "))
	println(downloadURL)

	return downloadURL
}

func getEmailLink(link string) string {
	releaseID := regexp.MustCompile(`tralbum_param\s*:\s*.*value\s*:\s*(\d*)`).FindStringSubmatch(releasePageHTML.FullText())[1]
	releaseType := regexp.MustCompile(`bandcamp.com\/?(track|album)\/`).FindStringSubmatch(link)[1]

	params := url.Values{}
	params.Add("agent", "Mozilla_foo_bar")
	params.Add("f", "get_email_address")
	params.Add("ip", "127.0.0.1")

	genEmailAddress, _ := soup.Get("http://api.guerrillamail.com/ajax.php?" + params.Encode())
	emailAddr := gjson.Get(genEmailAddress, "email_addr").String()
	sidToken := gjson.Get(genEmailAddress, "sid_token").String()

	baseURL, _ := url.Parse(link)
	baseURL.Path = "email_download"

	data := url.Values{}
	data.Add("address", emailAddr)
	data.Add("country", "US")
	data.Add("encoding_name", "none")
	data.Add("item_id", releaseID)
	data.Add("item_type", releaseType)
	data.Add("postcode", "20500")

	soup.PostForm(baseURL.String(), data)

	params.Set("f", "check_email")
	params.Add("sid_token", sidToken)
	params.Add("seq", "1")

	idFound := false
	var mailID string
	for !idFound {
		inboxData, _ := soup.Get("http://api.guerrillamail.com/ajax.php?" + params.Encode())
		if jsonSearch := gjson.Get(inboxData, "list.0.mail_id").String(); jsonSearch != "" {
			println()
			mailID = jsonSearch
			idFound = true
		} else {
			for _, icon := range []string{"-", "\\", "|", "/"} {
				color.New(color.FgCyan).Print("\r>>> WAITING " + icon)
				time.Sleep(250 * time.Millisecond)
			}
		}
	}

	params.Set("f", "fetch_email")
	params.Set("email_id", mailID)
	params.Del("sec")

	emailData, _ := soup.Get("http://api.guerrillamail.com/ajax.php?" + params.Encode())
	emailContentSoup := soup.HTMLParse(gjson.Get(emailData, "mail_body").String())

	params.Set("f", "forget_me")
	params.Del("email_id")
	params.Set("email_addr", emailAddr)
	soup.Get("http://api.guerrillamail.com/ajax.php?" + params.Encode())

	return emailContentSoup.Find("a").Attrs()["href"]
}

func getRetryURL(link string) string {
	selectDownloadPageHTML, _ := soup.Get(link)

	parsedURL, _ := url.Parse(link)
	urlQuerys := parsedURL.Query()
	releaseID := "id=" + urlQuerys.Get("id")

	releaseType := regexp.MustCompile(`download_type_str.*?(album|track)`).FindStringSubmatch(selectDownloadPageHTML)[1]
	fsig := regexp.MustCompile(`fsig=\w{32}\b`).FindString(selectDownloadPageHTML)
	ts := regexp.MustCompile(`ts=\w{10}.[0-9]\b`).FindString(selectDownloadPageHTML)

	if releaseID == "" || fsig == "" || ts == "" {
		return ""
	}

	color.New(color.FgCyan).Print(string(">>> "))
	println(fsig, releaseID, ts)

	popplersPage, _ := soup.Get(fmt.Sprintf("https://popplers5.bandcamp.com/statdownload/%s?enc=flac&%s&%s&%s", releaseType, fsig, releaseID, ts))
	jsonString := regexp.MustCompile(`{\".*\"}`).FindString(popplersPage)

	color.New(color.FgGreen).Print(string("==> "))
	println(gjson.Get(jsonString, "retry_url").String())

	return gjson.Get(jsonString, "retry_url").String()
}

func tryForFreeDownloadURL(link string) string {
	var downloadURL string
	freeDownloadPage := regexp.MustCompile(`freeDownloadPage\s*:\s*.*(["\'])(.+)(["\'])`).FindStringSubmatch(releasePageHTML.FullText())
	if freeDownloadPage == nil {
		selectDownloadURL := getEmailLink(link)
		downloadURL = getPopplersFromSelectDownloadPage(selectDownloadURL)
	} else {
		selectDownloadURL := freeDownloadPage[2]
		downloadURL = getRetryURL(selectDownloadURL)
	}
	if downloadURL == "" {
		color.Red("### Unable to get Retry URL")
		return ""
	}
	return downloadURL
}

func checkReleaseAvailability(link string) int {
	if regexp.MustCompile(`^(http?s:\/\/)?bandcamp.com\/.*$`).MatchString(link) {
		return 0
	}
	if regexp.MustCompile(`^(http?s:\/\/)?.+\.bandcamp.com\/?(music)?$`).MatchString(link) {
		return 1
	}
	if releasePageHTML.FindStrict("a", "class", "you-own-this-link").Error == nil {
		return 2
	}
	buyButton := releasePageHTML.FindStrict("h4", "class", "ft compound-button main-button")
	if buyButton.Error == nil {
		if strings.Contains(buyButton.FullText(), "name your price") {
			return 3
		} else if strings.Contains(buyButton.FullText(), "Free Download") {
			return 3
		}
	}
	return 4
}

func availAndDownload(releaseLink string) []string {
	var pathSlice []string
	releaseAvailability := checkReleaseAvailability(releaseLink)
	switch releaseAvailability {
	case 0:
		color.New(color.FgCyan).Print(string(">>> "))
		println("Getting links from User Profile")

		for _, releaseBox := range releasePageHTML.FindAll("li", "class", "collection-item-container") {
			if "https://bandcamp.com/"+config.UserName == releaseLink {
				releaseTitle := strings.TrimSpace(releaseBox.Find("div", "class", "collection-item-title").Text())
				releaseArtist := strings.TrimSpace(releaseBox.Find("div", "class", "collection-item-artist").Text())
				color.New(color.FgBlue).Print(string("--- "))
				println(releaseTitle, "by", releaseArtist)

				downloadPageLink := releaseBox.Find("span", "class", "redownload-item").Find("a").Attrs()["href"]
				color.New(color.FgGreen).Print(string("==> "))
				println(downloadPageLink)
				popplersLink := getPopplersFromSelectDownloadPage(downloadPageLink)
				downloadPath := download(popplersLink)

				pathSlice = append(pathSlice, downloadPath)
			}
			releasePageSoup, _ := soup.Get(releaseBox.Find("a", "class", "item-link").Attrs()["href"])
			releasePageHTML = soup.HTMLParse(releasePageSoup)
			availAndDownload(releaseBox.Find("a", "class", "item-link").Attrs()["href"])
		}
	case 1:
		color.New(color.FgCyan).Print(string(">>> "))
		println("Getting links from Artist Profile\n")

		releaseLink = regexp.MustCompile(`(.*com)`).FindString(releaseLink)

		releasePageSoup, _ := soup.Get(releaseLink + "/music")
		releasePageHTML = soup.HTMLParse(releasePageSoup)

		for _, boxLink := range releasePageHTML.Find("div", "class", "leftMiddleColumns").FindAll("a") {
			if strings.HasPrefix(boxLink.Attrs()["href"], "/") {
				color.New(color.FgGreen).Print(string("==> "))
				println(releaseLink + boxLink.Attrs()["href"])
				releasePageSoup, _ := soup.Get(releaseLink + boxLink.Attrs()["href"])
				releasePageHTML = soup.HTMLParse(releasePageSoup)
				availAndDownload(releaseLink + boxLink.Attrs()["href"])
			} else {
				color.New(color.FgGreen).Print(string("==> "))
				println(boxLink.Attrs()["href"])
				releasePageSoup, _ := soup.Get(boxLink.Attrs()["href"])
				releasePageHTML = soup.HTMLParse(releasePageSoup)
				availAndDownload(boxLink.Attrs()["href"])
			}
		}
	case 2:
		color.New(color.FgCyan).Print(string(">>> "))
		println("Getting link from User Profile\n")

		releaseTitle := strings.TrimSpace(releasePageHTML.Find("h2", "class", "trackTitle").Text())
		releaseArtist := strings.TrimSpace(releasePageHTML.Find("span", "itemprop", "byArtist").Find("a").Text())
		color.New(color.FgBlue).Print(string("--- "))
		println(releaseTitle, "by", releaseArtist)

		userProfileSoup, _ := soup.Get("https://bandcamp.com/" + config.UserName)
		userProfileHTML = soup.HTMLParse(userProfileSoup)
		releaseID := regexp.MustCompile(`tralbum_param\s*:.*?value\s*:\s*(\d*)`).FindStringSubmatch(releasePageHTML.FullText())[1]
		releaseBox := userProfileHTML.Find("li", "data-itemid", releaseID)

		downloadPageLink := releaseBox.Find("span", "class", "redownload-item").Find("a").Attrs()["href"]
		color.New(color.FgGreen).Print(string("==> "))
		println(downloadPageLink)
		popplersLink := getPopplersFromSelectDownloadPage(downloadPageLink)
		downloadPath := download(popplersLink)

		pathSlice = append(pathSlice, downloadPath)
	case 3:
		releaseTitle := strings.TrimSpace(releasePageHTML.Find("h2", "class", "trackTitle").Text())
		releaseArtist := strings.TrimSpace(releasePageHTML.Find("span", "itemprop", "byArtist").Find("a").Text())
		color.New(color.FgBlue).Print(string("--- "))
		println(releaseTitle, "by", releaseArtist)

		popplersLink := tryForFreeDownloadURL(releaseLink)
		downloadPath := download(popplersLink)

		pathSlice = append(pathSlice, downloadPath)
	case 4:
		if releasePageHTML.FindStrict("div", "id", "name-section").Error == nil {
			releaseTitle := strings.TrimSpace(releasePageHTML.Find("h2", "class", "trackTitle").Text())
			releaseArtist := strings.TrimSpace(releasePageHTML.Find("span", "itemprop", "byArtist").Find("a").Text())
			color.New(color.FgBlue).Print(string("--- "))
			println(releaseTitle, "by", releaseArtist)
			color.Red("### Paid Album\n\n")
		} else {
			color.Red("### Invalid Album\n\n")
		}
	}
	return pathSlice
}

func validateLink(link string) string {
	link = strings.TrimSpace(link)
	re := regexp.MustCompile(`https?:\/\/((.+\.bandcamp.com\/?.*)|(bandcamp.com\/.+))`)
	if validLink := re.FindStringSubmatch(link); validLink != nil {
		return "https://" + validLink[1]
	}
	return ""
}

func get(releaseLink string) []string {
	color.New(color.FgGreen).Print(string("==> "))
	println(releaseLink)

	releaseLink = validateLink(releaseLink)
	if releaseLink == "" {
		color.Red("### Invalid Link")
		return nil
	}

	releasePageSoup, _ := soup.Get(releaseLink)
	releasePageHTML = soup.HTMLParse(releasePageSoup)

	return availAndDownload(releaseLink)
}

func scanLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	return lines, nil
}

func printLogo() {
	logo := `
     __                        __  __
    /  |                      /  |/  |
    ## |____    _______   ____## |## |
    ##      \  /       | /    ## |## |
    #######  |/#######/ /####### |## |
    ## |  ## |## |      ## |  ## |## |
    ## |__## |## \_____ ## \__## |## |
    ##    ##/ ##       |##    ## |## |
    #######/   #######/  #######/ ##/`
	for _, c := range logo {
		if c == '#' {
			color.New(color.FgCyan).Print(string(c))
			continue
		}
		print(string(c))
	}
	print("\n\n")
}

// Config for login
var config = struct {
	Identity string
	UserName string
}{}

var (
	userProfileHTML        soup.Root
	releasePageHTML        soup.Root
	selectDownloadPageHTML soup.Root
)

func main() {
	printLogo()

	if _, err := os.Stat("config.yaml"); os.IsNotExist(err) {
		configExample := []byte("# Load bandcamp.com -> Inspect Element -> Application -> Cookies -> identity\nidentity: \n# Your profile name - bandcamp.com/(username)\nusername: ")
		ioutil.WriteFile("config.yaml", configExample, 0644)
	}

	configor.Load(&config, "config.yaml")

	if config.UserName == "" {
		config.UserName = "random"
	}

	soup.Cookie("identity", config.Identity)
	soup.Header("User-Agent", "Mozilla/5.0 (Windows NT 6.2 rv:20.0) Gecko/20121202 Firefox/20.0")

	var releaseLink string
	for {
		if len(os.Args) == 1 {
			reader := bufio.NewReader(os.Stdin)
			print("Input URL: ")
			releaseLink, _ = reader.ReadString('\n')
		} else {
			if strings.HasPrefix(os.Args[1], "-") {
				switch os.Args[1] {
				case "-b":
					if _, err := os.Stat("download_links.txt"); os.IsNotExist(err) {
						color.Blue("--- Created download_links.txt")
						os.Create("download_links.txt")
						os.Exit(0)
					}
					data, err := scanLines("download_links.txt")
					if err != nil {
						fmt.Println("File reading error", err)
						os.Exit(0)
					}
					for _, link := range data {
						get(link)
					}
					os.Exit(0)
				default:
					color.New(color.FgRed).Print(string("### "))
					println("bcdl [link]")
					os.Exit(0)
				}
			}
			releaseLink = os.Args[1]
		}

		get(releaseLink)
	}
}
