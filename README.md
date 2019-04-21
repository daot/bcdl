# bcdl
> Script to download free/pay what you want albums from Bandcamp

I was tired of scripts claiming to download "FLACs" from Bandcamp while in reality just ripping the 128kpbs preview streams the site uses. This script actually simulates "purchasing" the free/pay what you want albums and downloads them in the completely legal way that the average Joe would do at home, just faster and automated.

Currently, this script only runs with Selenium on Google Chrome because I haven't found a way to reliably download files with Firefox or other browsers.

**To Run:**
1. Install dependencies `BeautifulSoup4, Selenium`
2. Run with `python bcdl.py -e [email] -z [zip code] [url]`
