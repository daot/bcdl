import argparse
import os
import time

import requests
import wget
from bs4 import BeautifulSoup
from colorama import Fore, init
from selenium import webdriver
from selenium.common.exceptions import NoSuchElementException
from selenium.webdriver.common.keys import Keys


def dl(driver):
    driver.get(driver.current_url)
    driver.delete_all_cookies()
    driver.add_cookie({'name': 'download_encoding', 'value': '401'})
    driver.refresh()
    while not driver.find_element_by_css_selector("a.item-button").is_displayed():
        pass
    wget.download(driver.find_element_by_css_selector("a.item-button").get_attribute('href'), out="Downloads")
    print()


def free(driver):
    driver.find_element_by_xpath('//*[@id="trackInfoInner"]/ul/li[1]/div[2]/h4/button').click()
    try:
        driver.find_element_by_id('fan_email_address').send_keys(email)
        driver.find_element_by_id('fan_email_postalcode').send_keys(zip)
        driver.find_element_by_id('fan_email_postalcode').send_keys(Keys.RETURN)
        print(Fore.CYAN + "[Check Email]")
        return
    except NoSuchElementException:
        pass
    dl(driver)


def nyp(driver):
    driver.find_element_by_xpath('//*[@id="trackInfoInner"]/ul/li[1]/div[3]/h4[1]/button').click()
    driver.find_element_by_id('userPrice').send_keys("0")
    time.sleep(2)
    if "Alternatively, continue with zero" in driver.find_element_by_class_name('payment-nag-continue').text:
        driver.find_element_by_class_name('payment-nag-continue').click()
        driver.find_element_by_id('userPrice').send_keys(Keys.RETURN)
        try:
            driver.find_element_by_id('fan_email_address').send_keys(email)
            driver.find_element_by_id('fan_email_postalcode').send_keys(zip)
            driver.find_element_by_id('fan_email_postalcode').send_keys(Keys.RETURN)
            print(Fore.CYAN + "[Check Email]")
        except NoSuchElementException:
            pass
    dl(driver)


def downloadCheck(name, links):
    chromeOptions = webdriver.ChromeOptions()
    chromeOptions.add_argument("headless")
    chromeOptions.add_argument("log-level=2")
    chromeOptions.add_argument("blink-settings=imagesEnabled=false")
    driver = webdriver.Chrome(options=chromeOptions)

    for link in links:
        page = requests.get(f"http://{name}.bandcamp.com{link}")
        soup = BeautifulSoup(page.text, 'html.parser')
        print(' '.join(soup.find('h2', {'class': 'trackTitle'}).text.split()), end=' - ')
        try:
            if "name your price" in ' '.join(soup.find('h4', {'class': 'ft compound-button main-button'}).text.split()):
                print(Fore.YELLOW + "NYP")
                driver.get(f"http://{name}.bandcamp.com{link}")
                nyp(driver)
            elif "Free Download" in ' '.join(soup.find('h4', {'class': 'ft compound-button main-button'}).text.split()):
                print(Fore.GREEN + "FREE")
                driver.get(f"http://{name}.bandcamp.com{link}")
                free(driver)
            else:
                print(Fore.RED + "PAID")
        except AttributeError:
            print(Fore.MAGENTA + "UNAVALABLE")


def getLinks():
    name = url.replace('https://', '').replace('http://', '').split('.bandcamp')[0]
    data = requests.get(f"http://{name}.bandcamp.com/music")
    soup = BeautifulSoup(data.text, 'html.parser')
    links = []
    for album in soup.find_all('li', {'class': 'music-grid-item square first-four'}):
        for link in album.find_all('a', href=True):
            if link['href'].startswith('/'):
                links.append(link['href'])
    for album in soup.find_all('li', {'class': 'music-grid-item square'}):
        for link in album.find_all('a', href=True):
            if link['href'].startswith('/'):
                links.append(link['href'])
    downloadCheck(name, links)


if __name__ == '__main__':
    init(autoreset=True)
    parser = argparse.ArgumentParser(description='Download free albums from Bandcamp.')
    parser.add_argument('email', type=str, help='email to send links too')
    parser.add_argument('zip', type=int, help='zipcode')
    parser.add_argument('url', type=str, help='url of artist page')
    args = parser.parse_args()
    email = args.email
    zip = args.zip
    url = args.url

    if not os.path.isdir("Downloads"):
        os.mkdir("Downloads")

    getLinks()
