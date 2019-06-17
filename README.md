# bcdl
> Script to download free/pay what you want albums from Bandcamp

I was tired of scripts claiming to download "FLACs" from Bandcamp while in reality just ripping the 128kpbs preview streams the site uses. This script actually simulates "purchasing" the free/pay what you want albums and downloads them in the completely legal way that the average Joe would do at home, just faster and automated.

Currently, this script only runs with Selenium on Google Chrome.

It also does not download the links sent to your email. I don't feel like making a whole email server just for this project, but in theory could be easily™️ added.

**To Run:**
1. Install dependencies in requirements.txt `python -m pip install -r requirements.txt`
2. Run with `python bcdl.py -e [email] [-b/urls]`
