# bcdl
> Script to download free/pay what you want albums from Bandcamp

![](sc.jpg)

I was tired of scripts claiming to download "FLACs" from Bandcamp while in reality just ripping the 128kpbs MP3 preview streams the site uses. This script actually simulates "purchasing" the free/pay what you want albums and downloads them in the completely legal and free way that the average Joe would do at home, just faster and automated.

You can use the flag `-b` to batch download from `download_links.txt`

**Prerequisites:** Dependencies in requirements.txt `pip install -r requirements.txt`

**Syntax:** `python bcdl.py [links/-b]`
