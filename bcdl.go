package main

import (
	"archive/zip"
	"bufio"
	"fmt"
	"html"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/anaskhan96/soup"
	"github.com/cheggaaa/pb"
	"github.com/fatih/color"
	"github.com/jinzhu/configor"
	"github.com/tidwall/gjson"
	"gopkg.in/alecthomas/kingpin.v2"
)

func getReviews(releaseFolder string, releaseLink string) {
	parsedURL, _ := url.Parse(releaseLink)
	dataEmbed := getAttrJSON("data-embed")

	albumID := gjson.Get(dataEmbed, "tralbum_param.value").String()
	releaseType := gjson.Get(dataEmbed, "tralbum_param.name").String()

	// Get all reviews from release
	jsonstring, err := soup.Post(fmt.Sprintf(`https://%s/api/tralbumcollectors/2/reviews`, parsedURL.Host),
		"application/x-www-form-urlencoded",
		fmt.Sprintf(`{"tralbum_type":"%s","tralbum_id":%s,"token":"1:9999999999:9999999999:1:1:0","count":9999999999,"exclude_fan_ids":[]}`, string(releaseType[0]), albumID))
	if err != nil {
		panic(err)
	}

	prefix := "\n\n"
	if !writeDescription {
		prefix = ""
		writeToFile(filepath.Join(releaseFolder, "info.txt"), "")
	}

	f, err := os.OpenFile(filepath.Join(releaseFolder, "info.txt"), os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	// If there are reviews, write them
	if gjson.Get(jsonstring, "results.#").Int() != 0 {
		if _, err = f.WriteString(prefix + "# Reviews:"); err != nil {
			panic(err)
		}

		for _, k := range gjson.Get(jsonstring, "results").Array() {
			cleaned := removeWhiteSpace(strings.TrimSpace(html.UnescapeString(k.String())))
			review := "\n\n" + gjson.Get(cleaned, "name").String() +
				" (" + gjson.Get(cleaned, "username").String() + "): " +
				gjson.Get(cleaned, "why").String()
			if _, err = f.WriteString(review); err != nil {
				panic(err)
			}
		}
	}
}

// https://golangcode.com/writing-to-file/
func writeToFile(filename string, data string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.WriteString(file, data)
	if err != nil {
		return err
	}
	return file.Sync()
}

func getDescription(releaseFolder string) {
	description := releasePageHTML.Find("meta", "name", "description").Attrs()["content"]
	writeToFile(filepath.Join(releaseFolder, "info.txt"), strings.TrimSpace(html.UnescapeString(description)))
}

func finalAdditives(releaseFolder string, releaseLink string) {
	releasePageSoup, _ := soup.Get(releaseLink)
	releasePageHTML = soup.HTMLParse(releasePageSoup)

	if writeDescription {
		color.New(color.FgCyan).Print(string(">>> "))
		fmt.Println("Writing Description")
		getDescription(releaseFolder)
	}
	if writeReviews {
		color.New(color.FgCyan).Print(string(">>> "))
		fmt.Println("Writing Reviews")
		getReviews(releaseFolder, releaseLink)
	}
}

// https://stackoverflow.com/questions/20357223/easy-way-to-unzip-file-with-golang#24792688
func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

	os.MkdirAll(dest, 0755)

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		path := filepath.Join(dest, f.Name)

		// Check for ZipSlip (Directory traversal)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", path)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
		} else {
			os.MkdirAll(filepath.Dir(path), f.Mode())
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}

	return nil
}

func download(url string, releaseFolder string, filenamePrefix string) string {
retry:
	resp, err := http.Get(url)
	if err != nil {
		color.Red("### Unable to download")
		return ""
	}
	defer resp.Body.Close()

	_, params, _ := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	downloadPath := filepath.Join(outputFolder, filenamePrefix+params["filename"])

	if releaseFolder == "" {
		releaseFolder = strings.TrimSuffix(downloadPath, filepath.Ext(downloadPath))
	}

	if _, err := os.Stat(outputFolder); os.IsNotExist(err) {
		os.MkdirAll(outputFolder, 0755)
	}

	if overwrite {
		os.RemoveAll(downloadPath)
		os.RemoveAll(releaseFolder)
	}

	out, _ := os.Create(downloadPath)
	defer out.Close()

	i, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
	var sourceSize int64 = int64(i)

	if noBar {
		color.New(color.FgCyan).Print(string(">>> "))
		fmt.Print("Downloading")
		if _, err = io.Copy(out, resp.Body); err != nil {
			color.Red("### Error Saving")
			overwrite = true
			goto retry
		}
		fmt.Println(" - Done")
	} else {
		bar := pb.New(int(sourceSize)).SetUnits(pb.U_BYTES).SetRefreshRate(time.Millisecond * 10)
		bar.ShowSpeed = true
		bar.Start()

		reader := bar.NewProxyReader(resp.Body)

		if _, err = io.Copy(out, reader); err != nil {
			color.Red("### Error Saving")
			overwrite = true
			bar.Finish()
			goto retry
		}
		bar.Finish()
	}

	out.Close()
	if keepZip {
		return releaseFolder
	} else if filepath.Ext(downloadPath) != ".zip" {
		color.New(color.FgCyan).Print(string(">>> "))
		fmt.Println("Moving")
		os.MkdirAll(releaseFolder, 0755)
		err := os.Rename(downloadPath, filepath.Join(releaseFolder, filepath.Base(downloadPath)))
		if err != nil {
			log.Fatal(err)
		}
		os.Remove(downloadPath)
	} else {
		color.New(color.FgCyan).Print(string(">>> "))
		fmt.Print("Unzipping")
		unzip(downloadPath, releaseFolder)
		fmt.Println(" - Done")

		err = os.Remove(downloadPath)
		if err != nil {
			log.Panic(err)
		}
	}
	return releaseFolder
}

func getPopplersFromSelectDownloadPage(selectDownloadURL string) string {
	parsedURL, _ := url.Parse(selectDownloadURL)
	urlQuerys := parsedURL.Query()

	// Change some of the url parameters because email download links and collection download links are different.
	if urlQuerys.Get("from") == "collection" {
		selectDownloadPageHTML, _ := soup.Get(selectDownloadURL)
		urlQuerys.Set("id", regexp.MustCompile(`id=(\d*)&`).FindStringSubmatch(selectDownloadPageHTML)[1])
		urlQuerys.Set("type", regexp.MustCompile(`\/download\/(album|track)\?`).FindStringSubmatch(selectDownloadPageHTML)[1])
	}

	params := url.Values{}
	params.Add("enc", downloadQuality)
	params.Add("id", urlQuerys.Get("id"))
	params.Add("payment_id", urlQuerys.Get("payment_id"))
	params.Add("sig", urlQuerys.Get("sig"))
	params.Add(".rand", "1234567891234")
	params.Add(".vrs", "1")

	popplersPage, _ := soup.Get("https://popplers5.bandcamp.com/statdownload/" + urlQuerys.Get("type") + "?" + params.Encode())
	jsonString := regexp.MustCompile(`{\".*\"}`).FindString(popplersPage)
	downloadURL := gjson.Get(jsonString, "download_url").String()

	color.New(color.FgGreen).Print(string("==> "))
	fmt.Println(downloadURL)

	return downloadURL
}

func getRetryURL(link string) string {
	selectDownloadPageHTML, _ := soup.Get(link)

	parsedURL, _ := url.Parse(link)
	urlQuerys := parsedURL.Query()
	releaseID := "id=" + urlQuerys.Get("id")

	releaseType := regexp.MustCompile(`download_type_str.*?(album|track)`).FindStringSubmatch(selectDownloadPageHTML)[1]
	fsig := regexp.MustCompile(`fsig=\w{32}\b`).FindString(selectDownloadPageHTML)
	ts := regexp.MustCompile(`ts=\w{10}.[0-9]\b`).FindString(selectDownloadPageHTML)

	// All of the parts must be present for the request to work
	if releaseID == "" || fsig == "" || ts == "" {
		return ""
	}

	color.New(color.FgCyan).Print(string(">>> "))
	fmt.Println(fsig, releaseID, ts)

	popplersPage, _ := soup.Get(fmt.Sprintf("https://popplers5.bandcamp.com/statdownload/%s?enc=%s&%s&%s&%s", releaseType, downloadQuality, fsig, releaseID, ts))
	jsonString := regexp.MustCompile(`{\".*\"}`).FindString(popplersPage)

	color.New(color.FgGreen).Print(string("==> "))
	fmt.Println(gjson.Get(jsonString, "retry_url").String())

	return gjson.Get(jsonString, "retry_url").String()
}

func getEmailLink(releaseLink string) string {
	dataEmbed := getAttrJSON("data-embed")
	releaseID := gjson.Get(dataEmbed, "tralbum_param.value").String()
	releaseType := gjson.Get(dataEmbed, "tralbum_param.name").String()

	// Set email and clear inbox (manually doing request since soup doesn't support DELETE requests)
	emailAddr := "bcdl-" + strconv.Itoa(rand.Intn(10000)) + "@getnada.com"
	req, _ := http.NewRequest("DELETE", "https://getnada.com/api/v1/inboxes/"+emailAddr, nil)
	Client := &http.Client{}
	Client.Do(req)

	baseURL, _ := url.Parse(releaseLink)

	baseURL.Path = "email_download"
	data := url.Values{}
	data.Add("address", emailAddr)
	data.Add("country", "US")
	data.Add("encoding_name", "none")
	data.Add("item_id", releaseID)
	data.Add("item_type", releaseType)
	data.Add("postcode", "20500")

	// Send request to get download link
	soup.PostForm(baseURL.String(), data)

	// Wait until the email has been received. Checks every second.
	var UID string
	for UID == "" {
		inboxData, _ := soup.Get("https://getnada.com/api/v1/inboxes/" + emailAddr)
		UID = gjson.Get(inboxData, "msgs.0.uid").String()
		for _, icon := range []string{"-", "\\", "|", "/"} {
			color.New(color.FgCyan).Print("\r>>> WAITING " + icon)
			time.Sleep(250 * time.Millisecond)
		}
	}
	fmt.Println()

	// Get content of the email
	emailData, _ := soup.Get("https://getnada.com/api/v1/messages/html/" + UID)
	emailContentSoup := soup.HTMLParse(emailData)
	return emailContentSoup.Find("a").Attrs()["href"]
}

func getAttrJSON(attr string) string {
	r := regexp.MustCompile(attr + `=["\']({.+?})["\']`).FindStringSubmatch(html.UnescapeString(releasePageHTML.HTML()))[1]
	return r
}

func tryForFreeDownloadURL(releaseLink string) {
	// Searching for direct link to free download page. If mmissing, the release may be behind an email wall
	var downloadURL string
	var releaseFolder string
	tralbum := getAttrJSON("data-tralbum")
	freeDownloadPage := gjson.Get(tralbum, "freeDownloadPage").String()
	if freeDownloadPage == "" {
		releasePageSoup, _ := soup.Get(releaseLink)
		releasePageHTML = soup.HTMLParse(releasePageSoup)

		selectDownloadURL := getEmailLink(releaseLink)
		downloadURL = getPopplersFromSelectDownloadPage(selectDownloadURL)
		releaseFolder = download(downloadURL, "", "")
	} else {
		selectDownloadURL := freeDownloadPage
		downloadURL = getRetryURL(selectDownloadURL)
		releaseFolder = download(downloadURL, "", "")
	}
	finalAdditives(releaseFolder, releaseLink)
}

func freePageDownload(releaseLink string) {
	nameSection := releasePageHTML.Find("div", "id", "name-section")
	releaseTitle := strings.TrimSpace(nameSection.Find("h2", "class", "trackTitle").Text())
	releaseArtist := strings.TrimSpace(nameSection.Find("span").Find("a").Text())
	printReleaseName(releaseTitle, releaseArtist)

	overwrite = o
	if !o && !checkIfOverwrite(releaseTitle, releaseArtist) {
		fmt.Println()
		return
	}

	tryForFreeDownloadURL(releaseLink)
	fmt.Println()
}

func purchasedPageDownload(releaseLink string) {
	nameSection := releasePageHTML.Find("div", "id", "name-section")
	releaseTitle := strings.TrimSpace(nameSection.Find("h2", "class", "trackTitle").Text())
	releaseArtist := strings.TrimSpace(nameSection.Find("span").Find("a").Text())
	printReleaseName(releaseTitle, releaseArtist)

	overwrite = o
	if !o && !checkIfOverwrite(releaseTitle, releaseArtist) {
		fmt.Println()
		return
	}

	tralbum := getAttrJSON("data-tralbum")
	albumID := gjson.Get(tralbum, "id")

	if collectionSummary == "" {
		collectionSummary = getCollectionSummary(true)
	}
	redownloadMap := organizeRedownloadURLS(collectionSummary)

	salesID := gjson.Get(collectionSummary, fmt.Sprintf(`items.#(item_id=="%s").sale_item_id`, albumID))
	downloadPageLink := redownloadMap[salesID.String()]

	color.New(color.FgGreen).Print(string("==> "))
	fmt.Println(downloadPageLink)

	popplersLink := getPopplersFromSelectDownloadPage(downloadPageLink)
	releaseFolder := download(popplersLink, "", "")
	finalAdditives(releaseFolder, releaseLink)
	fmt.Println()
}

func removeDuplicateValues(stringSlice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range stringSlice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func artistPageLinkGen(releaseLink string) {
	u, _ := url.Parse(releaseLink)
	u.Path = "/music"
	releasePageSoup, _ := soup.Get(u.String())
	releasePageHTML = soup.HTMLParse(releasePageSoup)

	if scriptJSON := regexp.MustCompile(`application\/ld\+json["\']>\s+({.+?})\s+<`).FindStringSubmatch(html.UnescapeString(releasePageHTML.HTML())); scriptJSON != nil {
		get(gjson.Get(scriptJSON[1], "@id").String())
		return
	}

	var pageLinks []string
	u, err := url.Parse(releaseLink)
	if err != nil {
		log.Fatal(err)
	}

	// Gets all links in the middle box
	for _, boxLink := range releasePageHTML.Find("div", "class", "leftMiddleColumns").FindAll("a") {
		rel, err := u.Parse(boxLink.Attrs()["href"])
		if err != nil {
			log.Fatal(err)
		}
		pageLinks = append(pageLinks, rel.String())
	}

	// Removes duplicate values from pages that have featured releases
	pageLinks = removeDuplicateValues(pageLinks)

	// Sending individual links to get retested
	for _, pageLink := range pageLinks {
		get(pageLink)
	}
}
func getCollectionSummary(self bool) string {
	// Get fan ID
	var fanID string
	if self {
		collectionSummary, err := soup.Get("https://bandcamp.com/api/fan/2/collection_summary")
		if err != nil {
			panic(err)
		}
		fanID = gjson.Get(collectionSummary, "fan_id").String()
	} else {
		buttonid := releasePageHTML.Find("div", "id", "following-actions").Find("button").Attrs()["id"]
		fanID = regexp.MustCompile(`follow-unfollow_(\d*)`).FindStringSubmatch(buttonid)[1]
	}

	// Get all redownload urls from profile
	jsonstring, err := soup.Post("https://bandcamp.com/api/fancollection/1/collection_items",
		"application/x-www-form-urlencoded",
		fmt.Sprintf(`{"fan_id":%s,"older_than_token":"9999999999::a::","count":9999999999}`, fanID))
	if err != nil {
		panic(err)
	}
	return jsonstring
}

func removeWhiteSpace(s string) string {
	// Removes a variety of white space types. Probably breaks script languages
	out := regexp.MustCompile(`^[\s\p{Zs}\p{Cf}]+|[\s\p{Zs}\p{Cf}]+$`).ReplaceAllString(strings.TrimSpace(s), "")
	out = regexp.MustCompile(`[\s\p{Zs}\p{Cf}]{2,}`).ReplaceAllString(out, " ")
	return out
}

func printReleaseName(releaseTitle string, releaseArtist string) {
	color.New(color.FgBlue).Print(string("--- "))
	fmt.Println(removeWhiteSpace(releaseTitle), "by", removeWhiteSpace(releaseArtist))
}

func checkIfOverwrite(releaseTitle string, releaseArtist string) bool {
	releasePath := filepath.Join(outputFolder, fmt.Sprintf("%s - %s", releaseArtist, releaseTitle))
	_, err := os.Stat(releasePath)
	releasePath2 := filepath.Join(outputFolder, fmt.Sprintf("%s - %s", releaseArtist, releaseTitle)+".zip")
	_, err2 := os.Stat(releasePath2)
	if !os.IsNotExist(err) || !os.IsNotExist(err2) {
		var choice string
		for strings.ToLower(choice) != "y" && strings.ToLower(choice) != "n" {
			color.New(color.FgGreen).Print(string("==> "))
			fmt.Print("Would you like to redownload this release? (y/n): ")
			fmt.Scanln(&choice)

			if choice == "n" {
				return false
			}
		}
	}
	return true
}

func organizeRedownloadURLS(jsonstring string) map[string]string {
	redownloadJSON := gjson.Get(jsonstring, "redownload_urls")
	redownloadMap := make(map[string]string)

	// Get map of correct sales IDs
	for k, v := range redownloadJSON.Map() {
		if strings.HasPrefix(k, "p") || strings.HasPrefix(k, "c") {
			redownloadMap[k[1:]] = v.String()
		} else {
			u, err := url.Parse(v.String())
			urlQuerys := u.Query()
			if err != nil {
				log.Fatal(err)
			}
			redownloadMap[urlQuerys.Get("sitem_id")] = v.String()
		}
	}

	return redownloadMap
}

func userPageLinkGen(releaseLink string) {
	// Decide if the user page entered is the user's or another's
	if config.UserName == regexp.MustCompile(`.*bandcamp.com/(.*)`).FindStringSubmatch(releaseLink)[1] {
		collectionSummary = getCollectionSummary(true)

		for _, k := range gjson.Get(collectionSummary, "items").Array() {
			printReleaseName(gjson.Get(k.Raw, "album_title").String(),
				gjson.Get(k.Raw, "band_name").String())

			redownloadMap := organizeRedownloadURLS(collectionSummary)

			redownloadLink := redownloadMap[gjson.Get(k.Raw, "sale_item_id").String()]
			if redownloadLink == "" {
				if strings.HasSuffix(gjson.Get(k.Raw, "item_url").String(), "/subscribe") {
					continue
				}
				get(gjson.Get(k.Raw, "item_url").String())
			} else {
				color.New(color.FgGreen).Print(string("==> "))
				fmt.Println(redownloadLink)

				popplersLink := getPopplersFromSelectDownloadPage(redownloadLink)
				releaseFolder := download(popplersLink, "", "")
				finalAdditives(releaseFolder, releaseLink)
				fmt.Println()
			}
		}
	} else {
		collectionSummary = getCollectionSummary(false)
		for _, k := range gjson.Get(collectionSummary, "items").Array() {
			if strings.HasSuffix(gjson.Get(k.Raw, "item_url").String(), "/subscribe") {
				continue
			}
			get(gjson.Get(k.Raw, "item_url").String())
		}
	}
}

// Finds out what type of release the link is
func checkReleaseAvailability(link string) int {
	// User page
	if releasePageHTML.Find("meta", "property", "og:type").Attrs()["content"] == "profile" {
		return 0
	}
	// Artist page
	if releasePageHTML.Find("meta", "property", "og:type").Attrs()["content"] == "band" {
		return 1
	}
	// Purchased
	if releasePageHTML.FindStrict("a", "class", "you-own-this-link").Error == nil {
		return 2
	}
	// Check if free download
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

// Directs the link to appropriate downloader
func availAndDownload(releaseLink string) {
	releaseAvailability := checkReleaseAvailability(releaseLink)
	switch releaseAvailability {
	case 0:
		color.New(color.FgCyan).Print(string(">>> "))
		fmt.Print("Getting links from User Page (May take a while)\n\n")
		userPageLinkGen(releaseLink)
	case 1:
		color.New(color.FgCyan).Print(string(">>> "))
		fmt.Print("Getting links from Artist Page (May take a while)\n\n")
		artistPageLinkGen(releaseLink)
	case 2:
		purchasedPageDownload(releaseLink)
	case 3:
		freePageDownload(releaseLink)
	default:
		color.Red("### Paid\n\n")
	}
}

// Makes sure url is valid Bandcamp link
func validateLink(link string) string {
	link = strings.TrimSpace(link)
	releasePageSoup, _ := soup.Get(link)
	releasePageHTML = soup.HTMLParse(releasePageSoup)
	if releasePageHTML.Find("meta", "property", "twitter:site").Attrs()["content"] == "@bandcamp" {
		u, _ := url.Parse(link)
		return string(u.String())
	}
	return ""
}

// Validate link, get link page, continues, returns downloaded file path
func get(releaseLink string) {
	color.New(color.FgGreen).Print(string("==> "))
	fmt.Println(releaseLink)

	releaseLink = validateLink(releaseLink)
	if releaseLink == "" {
		color.Red("### Invalid Link\n\n")
		return
	}
	availAndDownload(releaseLink)
}

// Reads file and returns lines as array
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

// Prints logo at startup
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
		} else {
			fmt.Print(string(c))
		}
	}
	fmt.Print("\n\n")
}

// Config for login
var config = struct {
	Identity string
	UserName string
}{}

// Common global variables
var (
	releasePageHTML   soup.Root
	outputFolder      string
	downloadQuality   string
	collectionSummary string
	writeDescription  bool
	writeReviews      bool
	noBar             bool
	keepZip           bool
	overwrite         bool
	o                 bool
)

func main() {
	// Print logo. wow.
	printLogo()

	// If config file does not exist, create and fill with instructions
	if _, err := os.Stat("config.yaml"); os.IsNotExist(err) {
		configExample := []byte(
			"# Load bandcamp.com -> Inspect Element -> Application -> Cookies -> identity\n" +
				"identity: \n" +
				"# Your profile name - bandcamp.com/(username)\n" +
				"username: ")
		ioutil.WriteFile("config.yaml", configExample, 0644)
	}

	// Load config file data
	configor.Load(&config, "config.yaml")

	// Generate placeholder username if config username is blank
	if config.UserName == "" {
		rand.Seed(time.Now().UnixNano())
		config.UserName = fmt.Sprint(rand.Int())
	}

	// Set bandcamp login cookie and generic user agent
	soup.Cookie("identity", config.Identity)
	soup.Header("User-Agent", "Mozilla/5.0 (Windows NT 6.2 rv:20.0) Gecko/20121202 Firefox/20.0")

	// Get CLI flags
	rlArg := kingpin.Arg("url", "URL to Download").String()
	batch := kingpin.Flag("batch", "Download From download_links.txt").Short('b').Bool()
	zFlag := kingpin.Flag("zipped", "Keep albums in .zip format (don't extract)").Short('z').Bool()
	dqFlag := kingpin.Flag("quality", "Quality of Download (mp3-v0, mp3-320, flac, aac-hi, vorbis, alac, wav, aiff-lossless)").Default("flac").Short('q').String()
	ofFlag := kingpin.Flag("output", "Output Folder").Default("downloads").Short('o').String()
	wdFlag := kingpin.Flag("description", "Download and write description to info.txt").Short('d').Bool()
	wrFlag := kingpin.Flag("reviews", "Download and write reviews to info.txt").Short('r').Bool()
	nbFlag := kingpin.Flag("nobar", "Turns off progress bar").Short('p').Bool()
	ovrFlag := kingpin.Flag("overwrite", "Does not ask if you want to overwrite a download").Short('f').Bool()
	kingpin.Parse()

	// Assign flags to variables
	releaseLink := *rlArg
	keepZip = *zFlag
	downloadQuality = *dqFlag
	outputFolder = *ofFlag
	writeDescription = *wdFlag
	writeReviews = *wrFlag
	noBar = *nbFlag
	o = *ovrFlag

	if *batch {
		// Batch downloading
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

		for i := 0; i < len(data); i++ {
			get(data[i])
		}
	} else {
		if releaseLink != "" {
			get(releaseLink)
		} else {
			color.Red("### Please enter a URL")
		}
	}
}
