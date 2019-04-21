import requests, os, time, sys
from selenium import webdriver
from selenium.webdriver.common.keys import Keys
from bs4 import BeautifulSoup

try:
    email = sys.argv[sys.argv.index("-e") + 1]
except:
    print("Please enter an email for sending links to using '-e'")
    exit()

try:
    zip = sys.argv[sys.argv.index("-z") + 1]
except:
    print("Please enter a zip code using '-z'")
    exit()

def dl(driver):
    current = driver.current_url

    chromeOptions = webdriver.ChromeOptions()
    prefs = {"download.default_directory" : f"{os.path.dirname(os.path.abspath(__file__))}\\Downloads"}
    chromeOptions.add_experimental_option("prefs",prefs)
    nd = webdriver.Chrome(chrome_options=chromeOptions)

    nd.get(current)

    nd.execute_script('''(function () {
        'use strict';

        var format = "FLAC";

        var selectedFormat = false;

        setTimeout(function(){
            var interval = setInterval(function () {
                if(!selectedFormat){
                    document.getElementsByClassName('item-format button')[0].click();
                    var spans = document.getElementsByTagName("span");

                    for (var i = 0; i < spans.length; i++) {
                      if (spans[i].textContent == format) {
                        spans[i].parentElement.click();
                        selectedFormat = true;
                        break;
                      }
                    }
                }else{
                    var errorText = document.getElementsByClassName("error-text")[0];
                    if (errorText.offsetParent !== null) {
                        location.reload();
                    }
                    try {
                        var maintenanceLink = document.getElementsByTagName("a")[0];
                        if (a.href.indexOf("bandcampstatus") > 0) {
                            location.reload();
                        }
                    } catch (e) { }
                    var titleLabel = document.getElementsByClassName('download-title')[0];
                    if (titleLabel.children[0].href !== undefined && titleLabel.children[0].href.length > 0) {
                        titleLabel.children[0].click();
                        clearTimeout(interval);
                    }
                }
            }, 2000);
        }, 2000);
    })();''')

def free(driver):
    driver.find_element_by_xpath('//*[@id="trackInfoInner"]/ul/li[1]/div[2]/h4/button').click()
    try:
        driver.find_element_by_id('fan_email_address').send_keys(email)
        driver.find_element_by_id('fan_email_postalcode').send_keys(zip)
        driver.find_element_by_id('fan_email_postalcode').send_keys(Keys.RETURN)
        return
    except:
        pass
    dl(driver)

def buy(driver):
    driver.find_element_by_xpath('//*[@id="trackInfoInner"]/ul/li[1]/div[3]/h4[1]/button').click()
    try:
        if "(no minimum)" in driver.find_element_by_xpath('//*[@id="fan_email"]/div[1]/div[1]/div[1]/span/span[2]/span').text:
            driver.find_element_by_id('userPrice').send_keys("0")
            time.sleep(2)
            if "Alternatively, continue with zero" in driver.find_element_by_class_name('payment-nag-continue').text:
                driver.find_element_by_class_name('payment-nag-continue').click()
                driver.find_element_by_id('userPrice').send_keys(Keys.RETURN)
                try:
                    driver.find_element_by_id('fan_email_address').send_keys(email)
                    driver.find_element_by_id('fan_email_postalcode').send_keys(zip)
                    driver.find_element_by_id('fan_email_postalcode').send_keys(Keys.RETURN)
                    return
                except:
                    pass
                dl(driver)
    except:
        pass

if not os.path.isdir("Downloads"):
    os.mkdir("Downloads")
name = str(sys.argv[-1]).replace('https://', '').replace('http://', '').split('.bandcamp')[0]
data = requests.get(f"http://{name}.bandcamp.com/music")
soup = BeautifulSoup(data.text, 'html.parser')

links = []
for album in soup.find_all('li', {'class': 'music-grid-item square'}):
    for link in album.find_all('a', href=True):
        if link['href'].startswith('/'):
            links.append(link['href'])

chromeOptions = webdriver.ChromeOptions()
prefs = {"download.default_directory" : f"{os.path.dirname(os.path.abspath(__file__))}\\Downloads"}
chromeOptions.add_experimental_option("prefs",prefs)
driver = webdriver.Chrome(chrome_options=chromeOptions)

for link in links:
    driver.get(f"http://{name}.bandcamp.com{link}")
    try:
        driver.find_element_by_xpath('//*[@id="trackInfoInner"]/ul/li[1]/div[2]/h4/button')
        f = True
    except:
        try:
            driver.find_element_by_xpath('//*[@id="trackInfoInner"]/ul/li[1]/div[3]/h4[1]/button')
            f = False
        except:
            continue

    if f:
        free(driver)
    else:
        buy(driver)

driver.close()
