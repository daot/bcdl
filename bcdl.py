import requests, os, time, sys, argparse
from selenium import webdriver
from selenium.webdriver.common.keys import Keys
from selenium.common.exceptions import NoSuchElementException
from bs4 import BeautifulSoup

def dl(driver):
    chromeOptions = webdriver.ChromeOptions()
    prefs = {"download.default_directory" : os.path.join(os.path.dirname(os.path.abspath(__file__)), "Downloads")}
    chromeOptions.add_experimental_option("prefs", prefs)
    chromeOptions.add_experimental_option("detach", True)
    nd = webdriver.Chrome(options=chromeOptions)

    nd.get(driver.current_url)

    nd.delete_all_cookies()
    nd.add_cookie({'name': 'download_encoding', 'value': '401'})
    nd.refresh()
    while nd.find_element_by_css_selector("a.item-button").is_displayed() == False:
        pass
    nd.find_element_by_css_selector("a.item-button").click()

def free(driver):
    driver.find_element_by_xpath('//*[@id="trackInfoInner"]/ul/li[1]/div[2]/h4/button').click()
    try:
        driver.find_element_by_id('fan_email_address').send_keys(email)
        driver.find_element_by_id('fan_email_postalcode').send_keys(zip)
        driver.find_element_by_id('fan_email_postalcode').send_keys(Keys.RETURN)
        print("[Check Email]")
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
            print("[Check Email]")
        except NoSuchElementException:
            pass
    dl(driver)

def downloadCheck(name, links):
    driver = webdriver.Chrome()
    for link in links:
        page = requests.get(f"http://{name}.bandcamp.com{link}")
        soup = BeautifulSoup(page.text, 'html.parser')
        print(' '.join(soup.find('h2', {'class': 'trackTitle'}).text.split()), end=' - ')
        try:
            if "name your price" in ' '.join(soup.find('h4', {'class': 'ft compound-button main-button'}).text.split()):
                print("NYP")
                driver.get(f"http://{name}.bandcamp.com{link}")
                nyp(driver)
            elif "Free Download" in ' '.join(soup.find('h4', {'class': 'ft compound-button main-button'}).text.split()):
                print("FREE")
                driver.get(f"http://{name}.bandcamp.com{link}")
                free(driver)
            else:
                print("PAID")
        except AttributeError:
            print("UNAVALABLE")

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
    if not os.path.isdir("Downloads"):
        os.mkdir("Downloads")

    parser = argparse.ArgumentParser(description='Download free albums from Bandcamp.')
    parser.add_argument('email', type=str, help='email to send links too')
    parser.add_argument('zip', type=int, help='zipcode')
    parser.add_argument('url', type=str, help='url of artist page')

    args = parser.parse_args()

    email = args.email
    zip = args.zip
    url = args.url
    getLinks()
