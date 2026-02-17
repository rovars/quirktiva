import os
import sys
import subprocess
import urllib.request
import urllib.parse

def get_last_commits(n=5):
    try:
        output = subprocess.check_output(['git', 'log', '-n', str(n), '--pretty=format:- %s'], stderr=subprocess.STDOUT)
        return output.decode('utf-8')
    except Exception as e:
        return f"Error getting commits: {e}"

def send_document(base_url, chat_id, caption, file_path):
    if not file_path or not os.path.exists(file_path):
        print(f"::error::File not found or not provided: {file_path}")
        return

    print(f"Sending file: {file_path}")
    
    boundary = '----WebKitFormBoundary7MA4YWxkTrZu0gW'
    data = []
    # Simplified loop for parts
    parts = {
        'chat_id': chat_id,
        'caption': caption,
        'parse_mode': 'HTML'
    }
    for name, value in parts.items():
        data.append(f'--{boundary}')
        data.append(f'Content-Disposition: form-data; name="{name}"')
        data.append('')
        data.append(value)

    # File part
    data.append(f'--{boundary}')
    with open(file_path, 'rb') as f:
        data.append(f'Content-Disposition: form-data; name="document"; filename="{os.path.basename(file_path)}"')
        data.append('Content-Type: application/octet-stream')
        data.append('')
        data.append(f.read())
    data.append(f'--{boundary}--')
    data.append('')

    # Create the request body
    body = b''
    for i, item in enumerate(data):
        if isinstance(item, str):
            body += item.encode('utf-8') + b'\r\n'
        else:
            body += item + b'\r\n'

    req = urllib.request.Request(f"{base_url}/sendDocument", data=body)
    req.add_header('Content-Type', f'multipart/form-data; boundary={boundary}')
    
    try:
        with urllib.request.urlopen(req) as response:
            print(f"Result for {os.path.basename(file_path)}: {response.read().decode()}")
    except Exception as e:
        print(f"::error::Failed to send file {os.path.basename(file_path)}: {e}")
        # Optionally send a text-only message as a fallback
        # send_text_only(base_url, chat_id, f"Failed to upload {os.path.basename(file_path)}\n\n{caption}")

def send_text_only(base_url, chat_id, text):
    data = urllib.parse.urlencode({'chat_id': chat_id, 'text': text, 'parse_mode': 'HTML'}).encode()
    req = urllib.request.Request(f"{base_url}/sendMessage", data=data)
    try:
        with urllib.request.urlopen(req) as response:
            print(f"Result (Text): {response.read().decode()}")
    except Exception as e:
        print(f"::error::Failed to send text: {e}")

def main():
    bot_token = os.getenv('TELEGRAM_BOT_TOKEN')
    chat_id = os.getenv('TELEGRAM_CHAT_ID')
    
    if not bot_token or not chat_id:
        print("::error::TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID not set")
        sys.exit(1)

    files_to_send = sys.argv[1:]
    if not files_to_send:
        print("::error::No files provided to send.")
        sys.exit(1)

    commits = get_last_commits()
    commits_safe = commits.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
    caption = f"<b>- Build</b>\n{commits_safe}"
    base_url = f"https://api.telegram.org/bot{bot_token}"

    for file_path in files_to_send:
        send_document(base_url, chat_id, caption, file_path)

if __name__ == "__main__":
    main()
