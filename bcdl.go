package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"html"
	"io"
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

	"github.com/alecthomas/kingpin/v2"
	"github.com/anaskhan96/soup"
	"github.com/cheggaaa/pb"
	"github.com/fatih/color"
	"github.com/tidwall/gjson"
)

func sanitize(path string) string {
	//meant to mimic whatever bandcamp uses
	//probably not perfect
	return regexp.MustCompile(`[=,:<>[\]]+|[-,\.]+$`).ReplaceAllString(
		regexp.MustCompile(`[%*?|,/\\]`).ReplaceAllString(path, "-"),
		"")
}

func getDescription(releaseFolder string) {
	description := releasePageHTML.Find("meta", "name", "description").Attrs()["content"]

	d1 := []byte(strings.TrimSpace(html.UnescapeString(description)))
	err := os.WriteFile(filepath.Join(releaseFolder, "info.txt"), d1, 0644)
	if err != nil {
		log.Fatal("Error writing info.txt file", err)
	}
}

func finalAdditives(releaseFolder string, releaseLink string) {
	releasePageSoup, _ := soup.Get(releaseLink)
	releasePageHTML = soup.HTMLParse(releasePageSoup)

	if writeDescription {
		color.New(color.FgCyan).Print(string(">>> "))
		fmt.Println("Writing Description")
		getDescription(releaseFolder)
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
		fmt.Println(err)
		color.Red("### Unable to download")
		return ""
	}
	defer resp.Body.Close()

	_, params, _ := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	if params == nil {
		color.Red("### Artist out of Free Downloads")
		return ""
	}
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
	selectDownloadPageHTML, _ := soup.Get(selectDownloadURL)
	selectDownloadPageSoup := soup.HTMLParse(selectDownloadPageHTML)
	jsonString := selectDownloadPageSoup.Find("div","id","pagedata").Attrs()["data-blob"]
	downloadURL := gjson.Get(jsonString,"download_items.0.downloads."+downloadQuality+".url").String()

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

	// Generate temporary email address
	params := url.Values{}
	params.Add("agent", "Mozilla_foo_bar")
	params.Add("f", "get_email_address")
	params.Add("ip", "127.0.0.1")

	genEmailAddress, _ := soup.Get("http://api.guerrillamail.com/ajax.php?" + params.Encode())
	emailAddr := gjson.Get(genEmailAddress, "email_addr").String()
	sidToken := gjson.Get(genEmailAddress, "sid_token").String()

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

	params.Set("f", "check_email")
	params.Add("sid_token", sidToken)
	params.Add("seq", "1")

	// Wait until the email has been received. Checks every second.
	var mailID string
	for mailID == "" {
		inboxData, _ := soup.Get("http://api.guerrillamail.com/ajax.php?" + params.Encode())
		if jsonSearch := gjson.Get(inboxData, "list.0.mail_id").String(); jsonSearch != "" {
			fmt.Println()
			mailID = jsonSearch
		} else {
			for _, icon := range []string{"-", "\\", "|", "/"} {
				color.New(color.FgCyan).Print("\r>>> WAITING " + icon)
				time.Sleep(250 * time.Millisecond)
			}
		}
	}

	// Get content of the email
	params.Set("f", "fetch_email")
	params.Set("email_id", mailID)
	params.Del("sec")

	emailData, _ := soup.Get("http://api.guerrillamail.com/ajax.php?" + params.Encode())
	emailContentSoup := soup.HTMLParse(gjson.Get(emailData, "mail_body").String())

	// Forget the email account
	params.Set("f", "forget_me")
	params.Del("email_id")
	params.Set("email_addr", emailAddr)
	soup.Get("http://api.guerrillamail.com/ajax.php?" + params.Encode())

	return emailContentSoup.Find("a").Attrs()["href"]
}

func getAttrJSON(attr string) string {
	r := regexp.MustCompile(attr + `=["\']({.+?})["\']`).FindStringSubmatch(html.UnescapeString(releasePageHTML.HTML()))[1]
	return r
}

//print release name and check if the release should be downloaded
func preDownloadCheck(releaseTitle string, releaseArtist string) bool {
	printReleaseName(releaseTitle, releaseArtist)
	releasePath := findReleaseInFolder(releaseTitle, releaseArtist, outputFolder)
	
	if(skip>0){
		fmt.Println("Skipping...")
		skip--
		return false
	}
	if monitorFolder != "downloads" {
		monitoredRelease := findReleaseInFolder(releaseTitle, releaseArtist, monitorFolder)
		if monitoredRelease != "" {
			color.New(color.FgGreen).Print(string("==> "))
			fmt.Printf(`Found "%s" Moving to "%s"`, monitoredRelease, outputFolder)
			fmt.Print("\n\n")
			err := os.Rename(monitoredRelease, filepath.Join(outputFolder, filepath.Base(monitoredRelease)))
			if err != nil {
				panic(err)
			}
			return false
		}
	}
	if downloadQuality == "none" {
		fmt.Print("\n")
		return false
	}
	overwrite = true
	if o!="always" && !checkIfOverwrite(releasePath) {
		fmt.Println()
		overwrite = false
		return false
	}
	return true
}
func downloadRelease(releaseLink string, isPurchased bool) {
	nameSection := releasePageHTML.Find("div", "id", "name-section")
	releaseTitle := strings.TrimSpace(nameSection.Find("h2", "class", "trackTitle").Text())
	releaseArtist := strings.TrimSpace(nameSection.Find("span").Find("a").Text())
  
	if !preDownloadCheck(releaseTitle,releaseArtist){
		return
	}


	var downloadURL string
	var releaseFolder string
	tralbum := getAttrJSON("data-tralbum")
	if isPurchased {

		albumID := gjson.Get(tralbum, "id")

		if collectionSummary == "" {
			collectionSummary = getCollectionSummary(true)
		}
		redownloadMap := organizeRedownloadURLS(collectionSummary)

		salesID := gjson.Get(collectionSummary, fmt.Sprintf(`items.#(item_id=="%s").sale_item_id`, albumID))
		downloadPageLink := redownloadMap[salesID.String()]

		color.New(color.FgGreen).Print(string("==> "))
		fmt.Println(downloadPageLink)

		downloadURL = getPopplersFromSelectDownloadPage(downloadPageLink)
		releaseFolder = download(downloadURL, "", "")
	} else {
		// Free download logic
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
	}

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

	// Gets all links in the music grid
	musicGrid :=releasePageHTML.Find("ol","id","music-grid")

	for _, boxLink := range musicGrid.FindAll("a") {
		rel, err := u.Parse(boxLink.Attrs()["href"])
		if err != nil {
			log.Fatal(err)
		}
		pageLinks = append(pageLinks, rel.String())
	}

	// Check for javascript-rendered entries
	j := musicGrid.Attrs()["data-client-items"]
	var items []map[string]string
	json.Unmarshal([]byte(j), &items)
	for _, item := range items {
		rel, err := u.Parse(item["page_url"])
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

func findReleaseInFolder(releaseTitle string, releaseArtist string, searchFolder string) string {
	// Bandcamp downloads have the standard format of ArtistName - ReleaseTitle

	folderPath := sanitize(fmt.Sprintf("%s - %s", releaseArtist, releaseTitle))
	// Get a list of all files and subdirectories in the specified folder
	files, err := filepath.Glob(filepath.Join(searchFolder, "*"))
	if err != nil {
		return ""
	}

	// Iterate through the folders and check if the target filename exists
	for _, file := range files {
		base := filepath.Base(file)
		if strings.TrimSuffix(base, filepath.Ext(base)) == folderPath {
			// Found the folder, return its full path
			return file
		}
	}

	// If the folder is not found, check if a zip file with the same name exists
	for _, file := range files {
		base := filepath.Base(file)
		if base == (folderPath + ".zip") {
			// Found the file, return its full path
			return file
		}
	}

	// File not found in the folder
	return ""
}

func checkIfOverwrite(releasePath string) bool {
	_, err := os.Stat(releasePath)
	_, err2 := os.Stat(releasePath + ".zip")
	if !os.IsNotExist(err) || !os.IsNotExist(err2) {
		if o=="never" {
			fmt.Println("File already exists, skipping...")
			return false
		}
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
			if !preDownloadCheck(gjson.Get(k.Raw, "item_title").String(),
				gjson.Get(k.Raw, "band_name").String()){
					continue
				}

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
func checkReleaseAvailability() int {
	releaseType := releasePageHTML.Find("meta", "property", "og:type")
	if releaseType.Error != nil {
		return 4
	}
	// User page
	if releaseType.Attrs()["content"] == "profile" {
		return 0
	}
	// Artist page
	if releaseType.Attrs()["content"] == "band" {
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
	return 5
}

// if link requires payment write it in "paid.txt" file
func paidLink(link string) {
	f, err := os.OpenFile("paid.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer f.Close()

	if _, err = f.Write([]byte(link + "\n")); err != nil {
		fmt.Println(err)
		return
	}
}

// Directs the link to appropriate downloader
func availAndDownload(releaseLink string) {
	releaseAvailability := checkReleaseAvailability()
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
		downloadRelease(releaseLink, true)
	case 3:
		downloadRelease(releaseLink, false)
	case 4:
		color.Red("### Invalid Link\n\n")
	default:
		paidLink(releaseLink)
		color.Red("### Paid\n\n")
	}
}

// Validate link, get link page, continues, returns downloaded file path
func get(releaseLink string) {
	color.New(color.FgGreen).Print(string("==> "))
	fmt.Println(releaseLink)
	releaseLink = strings.TrimSpace(releaseLink)
	u, _ := url.Parse(releaseLink)
	releaseLink = string(u.String())
	releasePageSoup, _ := soup.Get(releaseLink)
	releasePageHTML = soup.HTMLParse(releasePageSoup)

	if releaseLink == "" {
		color.Red("### Invalid Link\n\n")
		return
	}
	availAndDownload(releaseLink)
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
	Identity string `json:"identity"`
	UserName string `json:"username"`
}{}

// Common global variables
var (
	releasePageHTML   soup.Root
	outputFolder      string
	monitorFolder     string
	downloadQuality   string
	collectionSummary string
	writeDescription  bool
	noBar             bool
	keepZip           bool
	overwrite         bool
	o                 string
	skip              int
)

func main() {
	// Read config json
	_, err := os.Stat("config.json")
	if err != nil {
		// Generate placeholder username if config username is blank
		config.UserName = strconv.Itoa(rand.Int())
		config.Identity = ""

	} else {
		// Config file exists, read it
		content, err := os.ReadFile("config.json")
		if err != nil {
			log.Fatal("File reading error", err)
		}
		err = json.Unmarshal(content, &config)
		if err != nil {
			log.Fatal("Error reading config", err)
		}
	}

	// Set bandcamp login cookie and generic user agent
	soup.Cookie("identity", config.Identity)
	soup.Header("User-Agent", "Mozilla/5.0 (Windows NT 6.2 rv:20.0) Gecko/20121202 Firefox/20.0")

	// Get CLI flags
	rlArg := kingpin.Arg("urls", "URLs to Download (Separated by space)").Strings()
	batch := kingpin.Flag("batch", "Download From [file]").Short('b').String()
	zFlag := kingpin.Flag("zipped", "Keep albums in .zip format (don't extract)").Short('z').Bool()
	dqFlag := kingpin.Flag("quality", "Quality of Download (mp3-v0, mp3-320, flac, aac-hi, vorbis, alac, wav, aiff-lossless) or 'none' to skip downloading entirely").Default("flac").Short('q').String()
	ofFlag := kingpin.Flag("output", "Output Folder").Default("downloads").Short('o').String()
	monFlag := kingpin.Flag("monitor", "If existing release is found in this folder, it will be moved to the downloads folder").Default("downloads").Short('m').String()
	wdFlag := kingpin.Flag("description", "Download and write description to info.txt").Short('d').Bool()
	nbFlag := kingpin.Flag("nobar", "Turns off progress bar").Short('p').Bool()
	logoFlag := kingpin.Flag("nologo", "Disables logo").Short('n').Bool()
	skipFlag := kingpin.Flag("skip", "Skip the first N downloads unconditionally").Short('s').PlaceHolder("N").Int()
	ovrFlag := kingpin.Flag("overwrite", "when to overwrite a download (always, ask, never)").PlaceHolder("WHEN").Default("ask").Enum("always", "ask", "never")
	
	kingpin.Parse()
	// Assign flags to variables
	releaseLinks := *rlArg
	keepZip = *zFlag
	downloadQuality = *dqFlag
	outputFolder = *ofFlag
	monitorFolder = *monFlag
	writeDescription = *wdFlag
	noBar = *nbFlag
	o = *ovrFlag
	logo := *logoFlag
	skip = *skipFlag

	if !logo {
		printLogo()
	}

	if *batch != "" {
		content, err := os.ReadFile(*batch)
		if err != nil {
			log.Fatal("File reading error", err)
		}
		lines := strings.Split(string(content), "\n")
		for i := 0; i < len(lines); i++ {
			get(lines[i])
		}
	} else {
		if releaseLinks == nil {
			color.Red("### Please enter a URL")
			os.Exit(1)
		}
		// Process release links separated by spaces for easier mass-downloading
		for _, element := range releaseLinks {
			get(element)
		}
	}
}
