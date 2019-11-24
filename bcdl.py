#!/usr/bin/python3

import json
import os
import re
import sys
import time
import urllib.parse

import requests
from bs4 import BeautifulSoup
from colorama import Fore, init
from tqdm import tqdm


def download(link):
    r = requests.get(link, stream=True, headers={'Range': 'bytes=0-', "User-Agent": "Mozilla/5.0 (Windows NT 6.2; rv:20.0) Gecko/20121202 Firefox/20.0"})
    r.raise_for_status()

    cd = r.headers.get('content-disposition')
    fn = urllib.parse.unquote(re.sub(r'(.*)UTF-8\'\'', '', cd))

    os.makedirs("Downloads", exist_ok=True)
    if os.path.isfile(os.path.join("Downloads", fn)):
        print(Fore.RED + "### EXISTS", end='\n\n')
        return False

    size = int(r.headers.get('content-length', 0))
    with open(os.path.join("Downloads", fn), 'wb') as f:
        with tqdm(total=size, unit='B',
                  unit_scale=True, ascii=True,
                  initial=0, miniters=1, dynamic_ncols=True) as bar:
            for chunk in r.iter_content(32 * 1024):
                if chunk:
                    f.write(chunk)
                    bar.update(len(chunk))
    print()
    return True


def get_download_link(url, q='flac'):
    # mp3-v0, mp3-320, flac, aac-hi, vorbis, alac, wav, aiff-lossless
    p = urllib.parse.parse_qs(urllib.parse.urlparse(url).query)
    d = {'enc': q, 'id': p['id'][0], 'payment_id': p['payment_id'][0], 'sig': p['sig'][0], '.rand': 1234567891234, '.vrs': 1}
    r = requests.get("https://popplers5.bandcamp.com/statdownload/{}?".format(p['type'][0]) + urllib.parse.urlencode(d))
    t = re.search(r'{\".*\"}', r.text).group()
    j = json.loads(t)
    print(Fore.GREEN + "-->", j['download_url'])
    download(j['download_url'])


def request_with_headers(url):
    user_agent = {"User-Agent": 'Mozilla/5.0 (Windows NT 6.2; rv:20.0) Gecko/20121202 Firefox/20.0'}
    return requests.get(url, headers=user_agent).text


def send_email(link, type):
    page = requests.get(link).text
    id = re.search(r'tralbum_param\s*:\s*([\{\}\,]).*value\s*:\s*(?P<param>(.*?)(?=[\}\s,]))', page).group('param')

    d = {'f': 'get_email_address', 'ip': '127.0.0.1', 'agent': 'Mozilla_foo_bar'}
    address = json.loads(request_with_headers('http://api.guerrillamail.com/ajax.php?' + urllib.parse.urlencode(d)))
    email = address['email_addr']
    sid = address['sid_token']

    profile = re.search(r'(?<=\/\/)[^\.]*', link).group()
    p = {"encoding_name": "none", "item_id": id, "item_type": type, "address": email, "country": "US", "postcode": "20500"}
    data = urllib.parse.urlencode(p).encode()
    requests.post("https://{}.bandcamp.com/email_download".format(profile), data=data)

    loaded = False
    while not loaded:
        try:
            d['f'] = 'check_email'
            d['sid_token'] = sid
            d['seq'] = 1
            j = json.loads(request_with_headers('http://api.guerrillamail.com/ajax.php?' + urllib.parse.urlencode(d)))
            mail_id = j['list'][0]['mail_id']
            print()
            loaded = True
        except IndexError:
            print('\r' + Fore.CYAN + ">>> WAITING -", end='')
            time.sleep(.25)
            print('\r' + Fore.CYAN + ">>> WAITING \\", end='')
            time.sleep(.25)
            print('\r' + Fore.CYAN + ">>> WAITING |", end='')
            time.sleep(.25)
            print('\r' + Fore.CYAN + ">>> WAITING /", end='')
            time.sleep(.25)

    d['f'] = 'fetch_email'
    del d['seq']
    d['email_id'] = mail_id
    j = json.loads(request_with_headers('http://api.guerrillamail.com/ajax.php?' + urllib.parse.urlencode(d)))['mail_body']

    d['f'] = 'forget_me'
    del d['email_id']
    d['email_addr'] = email
    request_with_headers('http://api.guerrillamail.com/ajax.php?' + urllib.parse.urlencode(d))

    soup = BeautifulSoup(j, 'html.parser')
    print(Fore.GREEN + "-->", soup.find('a')['href'])
    get_download_link(soup.find('a')['href'])


def get_retry(url):
    page = requests.get(url)
    type = re.search(r'[^type=]*(album|track)', url).group()
    fsig = re.search(r'fsig=\w{32}\b', page.text).group()
    bid = re.search(r'id=\w{10}\b', page.text).group()
    ts = re.search(r'ts=\w{10}.[0-9]\b', page.text).group()
    print(Fore.CYAN + ">>>", "{} {} {}".format(ts, fsig, bid))

    r = requests.get("https://popplers5.bandcamp.com/statdownload/{}?enc=flac&{}&{}&{}".format(type, fsig, bid, ts))
    t = re.search(r'{\".*\"}', r.text).group()
    j = json.loads(t)
    print(Fore.GREEN + "-->", j['retry_url'], end='\n')
    download(j['retry_url'])


def test_price(url, soup):
    try:
        if "Free Download" in soup.select("button.download-link.buy-link")[1].text:
            return "FREE"
        else:
            if "name your price" in soup.select("span.buyItemExtra.buyItemNyp.secondaryText")[0].text:
                return "FREE"
            else:
                return "PAID"
    except IndexError:
        return "UNV"


def link_type(link):
    if "/album/" in link:
        return "ALBUM"
    elif "/track/" in link:
        return "TRACK"
    elif 'bandcamp.com' in link:
        return "PAGE"


def get_free_page(link, type):
    print(Fore.GREEN + "==>", link)
    rq = requests.get(link)
    soup = BeautifulSoup(rq.text, 'html.parser')

    try:
        print(Fore.BLUE + "---", ' '.join(soup.find('div', {'id': 'name-section'}).text.replace('\n', ' ').strip().split()))
    except AttributeError:
        print(Fore.RED + "### INVALID LINK\n")
        return False

    t = test_price(link, soup)
    if t == "PAID":
        print(Fore.RED + "### PAID\n")
        return
    elif t == "UNV":
        print(Fore.RED + "### UNAVAILABLE\n")
        return
    elif t == "FREE":
        if re.search(r'freeDownloadPage\s*:\s*(["\'])(?P<url>(?:(?!\1).)+)\1', rq.text) is None:
            send_email(link, type)
        else:
            url = re.search(r'freeDownloadPage\s*:\s*(["\'])(?P<url>(?:(?!\1).)+)\1', rq.text).group('url')
            try:
                get_retry(url)
            except AttributeError:
                send_email(link, type)


def get(link):
    try:
        link = "https://" + re.search(r'(?<=\/\/)[^\.].*', link).group()
    except AttributeError:
        print(Fore.RED + "### INVALID LINK\n")
        return False

    test = link_type(link)
    name = re.search(r'(?<=\/\/)[^\.]*', link).group()

    if test == "ALBUM":
        get_free_page(link, "album")

    elif test == "TRACK":
        soup = BeautifulSoup(requests.get(link).text, 'html.parser')
        if soup.find("span", {"class": "fromAlbum"}) is None:
            get_free_page(link, "track")
        else:
            album = soup.find("span", {"class": "fromAlbum"}).parent['href']
            get("https://{}.bandcamp.com{}".format(name, album))

    elif test == "PAGE":
        soup = BeautifulSoup(requests.get("https://{}.bandcamp.com/music".format(name)).text, 'html.parser')
        if soup.find("div", {"class": "leftMiddleColumns"}) is not None:
            adiv = soup.find("div", {"class": "leftMiddleColumns"})
        else:
            print(Fore.RED + "### PAGE NOT FOUND\n")
            return False

        try:
            links = [a['href'] for a in adiv.find_all('a')]
        except KeyError:
            r = re.search(r'linkback\s*:\s*(["\'])(?P<page>(?:(?!\1).)+)\1\s*\+\s*\1(?P<url>(?:(?!\1).)+)\1', soup.text)
            links = [r.group('page') + r.group('url')]

        for link in links:
            if link.startswith('/album/'):
                get_free_page("https://{}.bandcamp.com{}".format(name, link), "album")
            elif link.startswith('/track/'):
                get_free_page("https://{}.bandcamp.com{}".format(name, link), "track")
            else:
                if "/album/" in link:
                    get_free_page(link.split("?")[0], "album")
                elif "/track/" in link:
                    get_free_page(link.split("?")[0], "track")
    else:
        print(Fore.RED + "### INVALID LINK\n")


def print_logo():
    logo = """
     __                        __  __
    /  |                      /  |/  |
    ## |____    _______   ____## |## |
    ##      \\  /       | /    ## |## |
    #######  |/#######/ /####### |## |
    ## |  ## |## |      ## |  ## |## |
    ## |__## |## \\_____ ## \\__## |## |
    ##    ##/ ##       |##    ## |## |
    #######/   #######/  #######/ ##/\n\n"""
    for char in logo:
        if char == "#":
            print(Fore.CYAN + '#', end='')
        else:
            print(char, end='')


if __name__ == '__main__':
    init(autoreset=True)

    print_logo()

    if len(sys.argv) > 1:
        if sys.argv[1] == "-b":
            if not os.path.isfile("download_links.txt"):
                open("download_links.txt", 'a', encoding='UTF-8').close()
                print(Fore.RED + "### download_links.txt NOT FOUND\n")
            else:
                with open('download_links.txt', 'r', encoding='UTF-8') as file:
                    f = file.readlines()
                    if len(f) == 0:
                        print()
                        print(Fore.RED + "### download_links.txt EMPTY\n")
                    else:
                        for link in f:
                            get(link.strip())
        else:
            for i in range(1, len(sys.argv)):
                get(sys.argv[i])
    else:
        while True:
            get(input("Input URL: "))
