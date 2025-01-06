# Don't Trust, Verify

In this tutorial, we will attempt to reproduce the results achieved with [Decentralized ChatGPT](https://docs.eternalai.org/eternal-ai/decentralized-inference-api/tutorials/decentralized-chatgpt) on our local machine (which supports a 4090 GPU, as the EternalAI platform currently uses a cluster of 4090 GPU machines to ensure determinism). This will help verify that the response is genuinely generated by the Prime Intellect 1 model. Let's proceed step by step.

## Step 1

Install Docker (if you haven't)

[https://docs.docker.com/engine/install/](https://docs.docker.com/engine/install/)

## Step 2

Create the `entrypoint.sh` file.

```bash
#!/bin/bash

# Start Ollama in the background.
/bin/ollama serve &
# Record Process ID.
pid=$!

# Pause for Ollama to start.
sleep 5

echo "🔴 Retrieve hf.co/lmstudio-community/INTELLECT-1-Instruct-GGUF:Q8_0 model..."
ollama run hf.co/lmstudio-community/INTELLECT-1-Instruct-GGUF:Q8_0
echo "🟢 Done!"

# Wait for Ollama process to finish.
wait $pid
```

## Step 3

Create the docker-compose.yml file.

```docker
version: '3.7'
services:
  ollama:
    image: ollama/ollama
    restart: always
    ports:
      - "11435:11434"
    volumes:
      - ./models:/app/models
      - ./entrypoint.sh:/entrypoint.sh
    entrypoint: ["/usr/bin/bash", "/entrypoint.sh"]
```

## Step 4

Run `docker compose up -d` to start Ollama.

It will download (if haven't) and then run the `hf.co/lmstudio-community/INTELLECT-1-Instruct-GGUF:Q8_0`model.

The download may take a while and you can check its progress by viewing log of the docker container by running `docker logs -f eternal-ollama-1` where `eternal-ollama-1` is container name.

It should be complete once you see the following log.

<figure><img src="../../.gitbook/assets/image (4).png" alt=""><figcaption></figcaption></figure>

## Step 5

Once the download is complete, a chat completion api will also be run (at port 11435) on your local machine that is ready for you to "chat" directly with the model and verify against the Decentralized ChatGPT's responses.

```bash
curl --location 'http://localhost:11435/v1/chat/completions' \
--header 'Content-Type: application/json' \
--header 'Authorization: Bearer ollama' \
--data '{
    "model": "PrimeIntellect/INTELLECT-1-Instruct",
    "max_tokens": 1024,
    "temperature": 0.6,
    "frequency_penalty": 0,
    "presence_penalty": 0,
    "messages": [
        {
            "role": "system",
            "content": "You are a helpfull assistant"
        },
        {
            "role": "user",
            "content": "what is btc?"
        }
    ]
}'
```

## Step 6

The response should look like:

```json
{
    "created": 1734583173,
    "model": "PrimeIntellect/INTELLECT-1-Instruct",
    "choices": [
        {
            "index": 0,
            "message": {
                "role": "assistant",
                "content": "Bitcoin (BTC) is a cryptocurrency and a digital asset that was created in 2009 by an individual or group of individuals using the pseudonym Satoshi Nakamoto. It is the first and most well-known cryptocurrency, and it is often referred to as the \"original\" or \"father\" of all cryptocurrencies.\n\nBitcoin is decentralized, meaning it is not controlled by any central authority or government. Instead, it is powered by a network of computers (nodes) that work together to validate and record transactions on a distributed ledger called the blockchain. The blockchain is a public ledger that is shared across the network and is immutable, meaning that once a transaction is recorded, it cannot be altered or deleted.\n\nBitcoin is also designed to be a deflationary asset, meaning that the total supply of Bitcoin is capped at 21 million coins, and the supply is slowly and predictably decreasing over time. This is in contrast to traditional currencies, which can be printed and issued by central banks at will.\n\nOne of the key features of Bitcoin is its ability to facilitate peer-to-peer transactions without the need for intermediaries like banks or payment processors. This makes it a more efficient and cost-effective way to transfer value over the internet.\n\nDespite its popularity and the attention it has received in the media, Bitcoin has also been the subject of controversy and criticism. Some critics argue that it is a speculative asset that is subject to extreme price volatility, and that it does not have a stable value. Others have raised concerns about its environmental impact, as the mining of Bitcoin requires significant amounts of energy.\n\nDespite these concerns, Bitcoin has continued to grow in popularity and has become a major player in the cryptocurrency market. It is currently the largest and most valuable cryptocurrency by market capitalization, and it is used by millions of people around the world for a variety of purposes, including as a means of exchange, a store of value, and a store of wealth."
            },
            "finish_reason": "stop"
        }
    ],
    "usage": {
        "prompt_tokens": 25,
        "completion_tokens": 385,
        "total_tokens": 410
    },
    "system_fingerprint": ""    
}
```

The response's `content` mentioned above should be deterministically identical to the content returned by EternalAI's decentralized inference API (used by the code from Decentralized ChatGPT post).