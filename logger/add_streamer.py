import psycopg2, requests, json, sys

config = json.load(open('../config.json'))

n = config["VIEWER_LIMIT"]
dbname = config["DN_NAME"]
user = config["DB_USER"]
password = config["DB_PASSWORD"]
client_id = config["CLIENT_ID"]
host = 'localhost'

try:
    conn = psycopg2.connect("""dbname='{0}' user='{1}' host='{2}' password='{3}'""".format(dbname, user, host, password))
except:
    e = sys.exc_info()[0]
    print("Not able to connext to db: %s" % e)

def insert_into_db(data):
    cur = conn.cursor()
    for stream in data["streams"]:
        if stream["channel"]["partner"]:
            command = '''INSERT INTO partner (id, language, name, view_count, follower_count, display_name) 
            VALUES ({0}, '{1}', '{2}', {3}, {4}, '{5}')
            ON CONFLICT (id) DO UPDATE 
              SET view_count = {3}, 
                  follower_count = {4},
                  language = '{1}',
                  display_name = '{5}',
                  name = '{2}';'''.format(stream["channel"]["_id"], 
                                          stream["channel"]["language"],
                                          stream["channel"]["name"], 
                                          stream["channel"]["views"], 
                                          stream["channel"]["followers"], 
                                          stream["channel"]["display_name"] )
            cur.execute(command)
    cur.close()
    conn.commit()

i = 0
viewers = n+1
while(viewers > n):
    j = requests.get("https://api.twitch.tv/kraken/streams?limit=25&offset={0}&stream_type=live&client_id={1}".format(i*25, client_id))
    data = json.loads(j.text)
    print(data["streams"][0]["viewers"])
    insert_into_db(data)
    i += 1
    viewers = data["streams"][0]["viewers"]

