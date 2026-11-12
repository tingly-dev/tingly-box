"""Small local MT backend for testing Tingly's native Hunyuan adapter.

Loads the 1.8B model through the Transformers Python SDK and exposes a minimal
OpenAI-compatible serving endpoint. Translation prompts are built by Tingly.
Bind to loopback; this development server does not implement authentication.
"""
import argparse
import threading
import time
import uuid

import torch
from tingly import Server
from transformers import AutoModelForCausalLM, AutoTokenizer


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--model", default="tencent/Hy-MT2-1.8B")
    parser.add_argument("--revision", help="Pin the Hub commit, or pass a local snapshot to --model")
    parser.add_argument("--served-model-name", help="Defaults to the model ID")
    parser.add_argument("--generic-chat", action="store_true", help="Allow system/user messages for a general instruction LLM")
    parser.add_argument("--port", type=int, default=18681)
    parser.add_argument("--dtype", choices=["float32", "bfloat16"], default="bfloat16")
    parser.add_argument("--max-new-tokens", type=int, default=128)
    args = parser.parse_args()
    args.served_model_name = args.served_model_name or args.model
    torch.set_num_threads(4)
    tokenizer = AutoTokenizer.from_pretrained(args.model, revision=args.revision)
    loaded_model = AutoModelForCausalLM.from_pretrained(
        args.model, revision=args.revision, dtype=getattr(torch, args.dtype),
        device_map="cpu", attn_implementation="eager",
    ).eval()
    lock = threading.Lock()
    srv = Server()

    @srv.openai_chat(args.served_model_name)
    def generate(messages, model=None, stream=False):
        if model != args.served_model_name or stream:
            raise ValueError("unsupported model or streaming")
        if args.generic_chat:
            if not messages or any(m.get("role") not in ("system", "user", "assistant") or not isinstance(m.get("content"), str) for m in messages):
                raise ValueError("expected text chat messages")
        elif len(messages) != 1 or messages[0].get("role") != "user" or not isinstance(messages[0].get("content"), str):
            raise ValueError("expected one user text message")
        with lock, torch.inference_mode():
            # Tokenize the rendered template explicitly for consistent SDK versions.
            prompt = tokenizer.apply_chat_template(messages, tokenize=False, add_generation_prompt=True)
            inputs = tokenizer(prompt, return_tensors="pt")
            n = inputs["input_ids"].shape[-1]
            if n + args.max_new_tokens > 8192:
                raise ValueError("input exceeds the model context budget")
            started = time.perf_counter()
            generated = loaded_model.generate(**inputs, max_new_tokens=args.max_new_tokens, do_sample=False)
            output = generated[0, n:]
            text = tokenizer.decode(output, skip_special_tokens=True)
            eos = loaded_model.generation_config.eos_token_id
            eos = [eos] if isinstance(eos, int) else (eos or [])
            finish = "stop" if len(output) < args.max_new_tokens or int(output[-1]) in eos else "length"
            print(f"generated {len(output)} tokens in {time.perf_counter()-started:.2f}s", flush=True)
            return {
                "id": "mt-" + uuid.uuid4().hex, "object": "chat.completion",
                "created": int(time.time()), "model": args.served_model_name,
                "choices": [{"index": 0, "message": {"role": "assistant", "content": text}, "finish_reason": finish}],
                "usage": {"prompt_tokens": n, "completion_tokens": len(output), "total_tokens": n + len(output)},
            }

    srv.run(host="127.0.0.1", port=args.port)


if __name__ == "__main__":
    main()
