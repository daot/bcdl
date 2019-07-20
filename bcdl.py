import argparse
import atexit
import os
import random
import string
import time

import requests
import wget
from bs4 import BeautifulSoup
from colorama import Fore, init
from selenium import webdriver
from selenium.common.exceptions import NoSuchElementException
from selenium.webdriver.common.keys import Keys


def dl(file):
    driver.delete_all_cookies()
    driver.add_cookie({'name': 'download_encoding', 'value': '401'})
    driver.refresh()
    while not driver.find_element_by_css_selector("a.item-button").is_displayed():
        pass
    wget.download(driver.find_element_by_css_selector("a.item-button").get_attribute('href'), out=os.path.join("Downloads", file))
    print()


def free(file):
    driver.find_element_by_css_selector("button.download-link.buy-link").click()
    try:
        driver.find_element_by_id('fan_email_address').send_keys(email)
        driver.find_element_by_id('fan_email_postalcode').send_keys("20500")
        driver.find_element_by_id('fan_email_postalcode').send_keys(Keys.RETURN)
        print(Fore.CYAN + "[Check Email]")
        return
    except NoSuchElementException:
        dl(file)


def nyp(file):
    driver.find_element_by_css_selector("button.download-link.buy-link").click()
    driver.find_element_by_id('userPrice').send_keys("0")
    time.sleep(2)
    if "Alternatively, continue with zero" in driver.find_element_by_class_name('payment-nag-continue').text:
        driver.find_element_by_class_name('payment-nag-continue').click()
        driver.find_element_by_id('userPrice').send_keys(Keys.RETURN)
        try:
            driver.find_element_by_id('fan_email_address').send_keys(email)
            driver.find_element_by_id('fan_email_postalcode').send_keys("20500")
            driver.find_element_by_id('fan_email_postalcode').send_keys(Keys.RETURN)
            print(Fore.CYAN + "[Check Email]")
        except NoSuchElementException:
            dl(file)


def downloadCheck(link):
    page = requests.get('https://' + (link.replace('https://', '').replace('http://', '')))
    soup = BeautifulSoup(page.text, 'html.parser')
    print(' '.join(soup.find('h2', {'class': 'trackTitle'}).text.split()), end=' - ')
    file = ' '.join(soup.find('span', {'itemprop': 'byArtist'}).text.split()) + ' - ' + ' '.join(soup.find('h2', {'class': 'trackTitle'}).text.split()) + '.zip'
    for ch in ['<', '>', ':', '"', "'", '/', '\\', '|', '?', '*']:
        if ch in file:
            file = file.replace(ch, '')
    if not os.path.isfile(os.path.join("Downloads", file)):
        try:
            if "name your price" in ' '.join(soup.find('h4', {'class': 'ft compound-button main-button'}).text.split()):
                print(Fore.YELLOW + "NYP")
                driver.get(link)
                nyp(file)
            elif "Free Download" in ' '.join(soup.find('h4', {'class': 'ft compound-button main-button'}).text.split()):
                print(Fore.GREEN + "FREE")
                driver.get(link)
                free(file)
            else:
                print(Fore.RED + "PAID")
        except AttributeError:
            print(Fore.MAGENTA + "UNAVALABLE")
    else:
        print(Fore.GREEN + "EXISTS")


def getLinks(link):
    name = link.replace('https://', '').replace('http://', '').split('.bandcamp')[0]
    data = requests.get(f"http://{name}.bandcamp.com/music")
    soup = BeautifulSoup(data.text, 'html.parser')
    links = []
    for album in soup.find_all('li', {'class': 'music-grid-item square first-four'}):
        for link in album.find_all('a', href=True):
            if link['href'].startswith('/'):
                links.append(f"http://{name}.bandcamp.com{link['href']}")
    for album in soup.find_all('li', {'class': 'music-grid-item square'}):
        for link in album.find_all('a', href=True):
            if link['href'].startswith('/'):
                links.append(f"http://{name}.bandcamp.com{link['href']}")
    cycle(links)


def cycle(links):
    try:
        for link in links:
            if "/album/" in link:
                downloadCheck(' '.join(link.split()))
            elif "/track/" in link:
                downloadCheck(' '.join(link.split()))
            else:
                getLinks(' '.join(link.split()))
    except TypeError:
        print(Fore.MAGENTA + "No links passed")


init(autoreset=True)

parser = argparse.ArgumentParser(description='Download free albums from Bandcamp.')
parser.add_argument('-e', type=str, help='email to send links too')
parser.add_argument('-b', action='store_true', help='download links in downloads.txt')
parser.add_argument('-l', type=str, help='links of bandcamp albums or text file',  nargs='+')
args = parser.parse_args()

if args.e is None:
    email = f"{''.join(random.choices(string.ascii_lowercase + string.digits, k=8))}@teleosaurs.xyz"
else:
    email = args.e
print(Fore.YELLOW + "All albums that need an email will be sent to:", Fore.RED + email)

chromeOptions = webdriver.ChromeOptions()
chromeOptions.add_argument("headless")
chromeOptions.add_argument("blink-settings=imagesEnabled=false")
chromeOptions.add_experimental_option('excludeSwitches', ['enable-logging'])
driver = webdriver.Chrome(options=chromeOptions)


def exit_handler():
    driver.quit()


atexit.register(exit_handler)

if not os.path.isdir("Downloads"):
    os.mkdir("Downloads")
if not os.path.isfile("downloads.txt"):
    open("downloads.txt", 'a', encoding='utf-8').close()

files = [f for f in os.listdir("Downloads") if os.path.isfile(os.path.join("Downloads", f))]

if args.b:
    with open('downloads.txt', 'r', encoding='utf-8') as file:
        cycle(file.readlines())
else:
    cycle(args.l)
