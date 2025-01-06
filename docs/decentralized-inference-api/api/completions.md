---
description: >-
  Given a prompt, the model will return one or more predicted completions along
  with the probabilities of alternative tokens at each position.
---

# Completions

## Create completion

Creates a completion for the provided prompt and parameters.&#x20;

```
POST https://api.eternalai.org/v1/completions
```

{% hint style="info" %}
We strive to keep EternalAI Completion API as similar to [OpenAI's ](https://platform.openai.com/docs/api-reference/completions/create)as possible, making it easy for developers who have built apps using OpenAI APIs to switch seamlessly.

The only differences are the inclusion of `chain_id` in the request body and `onchain_data` in the response body, as EternalAI APIs are empowered by a decentralized AI infrastructure.
{% endhint %}

### Request body

**model** `string` _Required_

* ID of the model to use.
* For additional details, refer to the [Onchain Models](https://docs.eternalai.org/eternal-ai/decentralized-inference-api/onchain-models).

**chain\_id** `string` _Optional Defaults to_ 45762 _(Symbiosis' chain id)_

* ID of blockchain hosting the model to use.
* For additional details, refer to the [Onchain Models](https://docs.eternalai.org/eternal-ai/decentralized-inference-api/onchain-models).

**prompt** `string or array`_Required_

* The prompt(s) to generate completions for, encoded as a string, array of strings, array of tokens, or array of token arrays.
* Note that <|endoftext|> is the document separator that the model sees during training, so if a prompt is not specified the model will generate as if from the beginning of a new document.

**best\_of** `integer or null` _Optional Defaults to 1_

* Generates `best_of` completions server-side and returns the "best" (the one with the highest log probability per token). Results cannot be streamed.
* When used with `n`, `best_of` controls the number of candidate completions and `n` specifies how many to return – `best_of` must be greater than `n`.
* **Note:** Because this parameter generates many completions, it can quickly consume your token quota. Use carefully and ensure that you have reasonable settings for `max_tokens` and `stop`.

**echo** `boolean or null` _Optional Defaults to false_

* Echo back the prompt in addition to the completion

**frequency\_penalty** _number or null_ _Optional Defaults to 0_

* Number between -2.0 and 2.0. Positive values penalize new tokens based on their existing frequency in the text so far, decreasing the model's likelihood to repeat the same line verbatim.

**logit\_bias** `map` _Optional Defaults to null_

* Modify the likelihood of specified tokens appearing in the completion.
* Accepts a JSON object that maps tokens (specified by their token ID in the GPT tokenizer) to an associated bias value from -100 to 100. You can use this tokenizer tool to convert text to token IDs. Mathematically, the bias is added to the logits generated by the model prior to sampling. The exact effect will vary per model, but values between -1 and 1 should decrease or increase likelihood of selection; values like -100 or 100 should result in a ban or exclusive selection of the relevant token.
* As an example, you can pass `{"50256": -100}` to prevent the <|endoftext|> token from being generated.

**logprobs** `integer or null` _Optional Defaults to null_

* Include the log probabilities on the `logprobs` most likely output tokens, as well the chosen tokens. For example, if `logprobs` is 5, the API will return a list of the 5 most likely tokens. The API will always return the `logprob` of the sampled token, so there may be up to `logprobs+1` elements in the response.
* The maximum value for `logprobs` is 5.

**max\_tokens** `integer or null` _Optional Defaults to 16_

* The maximum number of tokens that can be generated in the completion.
* The token count of your prompt plus `max_tokens` cannot exceed the model's context length.&#x20;

**n** `integer or null` _Optional Defaults to 1_

* How many completions to generate for each prompt.
* **Note:** Because this parameter generates many completions, it can quickly consume your token quota. Use carefully and ensure that you have reasonable settings for `max_tokens` and `stop`.

**presence\_penalty** `number or null` _Optional Defaults to 0_

* Number between -2.0 and 2.0. Positive values penalize new tokens based on whether they appear in the text so far, increasing the model's likelihood to talk about new topics.

**seed** `integer or null` _Optional_

* If specified, our system will make a best effort to sample deterministically, such that repeated requests with the same `seed` and parameters should return the same result.
* Determinism is not guaranteed, and you should refer to the `system_fingerprint` response parameter to monitor changes in the backend.

**stop** `string / array / null` _Optional Defaults to null_

* Up to 4 sequences where the API will stop generating further tokens. The returned text will not contain the stop sequence.

**stream** `boolean or null` _Optional Defaults to false_

* Whether to stream back partial progress. If set, tokens will be sent as data-only server-sent events as they become available, with the stream terminated by a `data: [DONE]` message.

**stream\_options** `object or null` _Optional Defaults to null_

* Options for streaming response. Only set this when you set `stream: true`.

**suffix** `string or null` _Optional Defaults to null_

* The suffix that comes after a completion of inserted text.

**temperature** `number or null` _Optional Defaults to 1_

* What sampling temperature to use, between 0 and 2. Higher values like 0.8 will make the output more random, while lower values like 0.2 will make it more focused and deterministic.
* We generally recommend altering this or `top_p` but not both.

**top\_p** `number or null` _Optional Defaults to 1_

* An alternative to sampling with temperature, called nucleus sampling, where the model considers the results of the tokens with top\_p probability mass. So 0.1 means only the tokens comprising the top 10% probability mass are considered.
* We generally recommend altering this or `temperature` but not both.

**user** `string` _Optional_

* A unique identifier representing your end-user, which can help EternalAI to monitor and detect abuse

### Response body

**id** `string`

* A unique identifier for the completion.

**choices** `array`

* The list of completion choices the model generated for the input prompt.

**created** `integer`

* The Unix timestamp (in seconds) of when the completion was created.

**model** `string`

* The model used for completion.

**system\_fingerprint** `string`

* This fingerprint represents the backend configuration that the model runs with.
* Can be used in conjunction with the `seed` request parameter to understand when backend changes have been made that might impact determinism.

**object** `string`

* The object type, which is always "text\_completion"

**usage** `object`

* Usage statistics for the completion request.

**onchain\_data** `object`

* **assignment\_addresses** `array`
  * addresses of model miners assigned to serve the inference
* **infer\_tx** `string`
  * tx hash of inference request tx
* **submit\_tx** `string`
  * tx hash of inference response tx submitted by a miner
* **input\_cid** `string`
  * content of inference prompt
* **output\_cid** `string`
  * content of inference response

### Example request & response

{% hint style="info" %}
The `ETERNALAI_API_KEY` can be obtained by following [the guide](https://docs.eternalai.org/eternal-ai/decentralized-inference-api/api-key)
{% endhint %}

#### Request

```bash
curl https://api.eternalai.org/v1/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ETERNALAI_API_KEY" \
  -d '{
    "chain_id":"45762",
    "model": "PrimeIntellect/INTELLECT-1-Instruct",
    "prompt": "Say this is a test",
    "max_tokens": 7,
    "temperature": 0
  }'
```

#### Response

```bash
{
    "id": "cmpl-7b18ee81f5574489acf3cd9e55eaee9e",
    "object": "text_completion",
    "created": 1732269168,
    "model": "PrimeIntellect/INTELLECT-1-Instruct",
    "choices": [
        {
            "text": "\nThis is indeed a test",
            "index": 0,
            "finish_reason": "length",
            "logprobs": {
                "tokens": null,
                "token_logprobs": null,
                "top_logprobs": null,
                "text_offset": null
            }
        }
    ],
    "usage": {
        "prompt_tokens": 6,
        "completion_tokens": 7,
        "total_tokens": 13
    },
    "onchain_data": {
        "pbft_committee": [
            "0xc2991c10413a7ceab23b3e31b6dac31df69ca23b",
            "0x6a2b25e22664d99421696a60adafa419e5c25b85",
            "0xba4a2bae114d07702d23c50cba7d12c764ccfece"
        ],
        "proposer": "0xc2991c10413a7ceab23b3e31b6dac31df69ca23b",
        "infer_tx": "0xc6214a3ea71d1849db3a31bf0adc5672565279cf47cc48127bf22bb9f461eef0",
        "propose_tx": "0xd26cf7d5f0265d0c3f05a36b7a268c7a913b9ea17e0656944c54da95ce12ccc8",
        "input_cid": "ipfs://bafkreiciaforvaqi5ciegyovr2fksbarlhmakmwquyjlp3gd6vnd4t5tyy",
        "output_cid": "ipfs://bafkreibszr27pwrui6x2wdbstshsnreqrehsvsplhbpho4c5ghvcfvpcbe"
    }
}
```