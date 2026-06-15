"""A minimal RAG-style experiment that reuses the tingly-box gateway.

Run a local tingly-box, then:

    pip install -e ".[all]"      # from sdk/python
    python examples/rag_experiment.py

Everything below routes through tb: provider selection, fallback, guard rails,
quota and logging are all applied for free. The experiment only owns its own
logic (here, a toy retriever).
"""

import tingly

# A stand-in "corpus". A real experiment would hit a vector store here.
CORPUS = {
    "tingly-box": "tingly-box is a personal intelligence orchestrator: an LLM "
    "gateway with remote control and guard rails.",
    "sdk": "The tingly Python SDK lets you write an experiment in a handful of "
    "lines and reuse the gateway.",
}


def retrieve(question: str) -> str:
    q = question.lower()
    hits = [text for key, text in CORPUS.items() if key in q]
    return "\n".join(hits) or "(no matching documents)"


def main() -> None:
    # Auto-discovers the local gateway and binds to the "experiment" scenario.
    with tingly.connect(scenario="experiment", name="rag-experiment") as tb:
        if not tb.ready:
            print(
                "Scenario 'experiment' has no active rule yet — bind one in the "
                "tingly-box UI, or run `tingly doctor`."
            )
            return

        question = "What is tingly-box?"
        docs = retrieve(question)
        answer = tb.ask(
            f"Using only these documents:\n{docs}\n\nAnswer: {question}",
            model="auto",
        )
        print("Q:", question)
        print("A:", answer)

        usage = tb.usage.this_session()
        print(f"\n[usage] {usage.requests} request(s), {usage.total_tokens} tokens")


if __name__ == "__main__":
    main()
