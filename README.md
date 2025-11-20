# RAG + SpiceDB Testcontainers Demo (Work in Progress)

This repository contains a **minimal end-to-end demo** showing how to test a **permission-aware RAG (Retrieval-Augmented Generation)** using **SpiceDB** running inside a **Testcontainer**.

The goal of the project is to demonstrate how a real authorization system — SpiceDB — can be embedded into automated tests to validate that your RAG pipeline only returns documents a user is allowed to see.

> ⚠️ **Work in Progress**  
> This project is still evolving. Code structure, APIs, and examples may change as the demo improves. Feedback and contributions are welcome!

---

## 🚀 What This Demo Shows

### ✔️ Spin up SpiceDB using Testcontainers  
Each test run creates a **fresh, isolated in-memory SpiceDB instance** using the community `testcontainers-spicedb-go` module.

### ✔️ Apply schema + relationships programmatically  
The test writes a small SpiceDB schema:

- `user`
- `document`
- `owner` and `viewer` relations  
- `read` permission (`owner + viewer`)

It also seeds sample relationships:

- Emilia owns `doc1`  
- Beatrice can view `doc2`  
- Everyone can view `doc3`

### ✔️ Run a sample RAG pipeline  
The RAG pipeline does:

1. **Trivial retrieval** (string match)  
2. **Post-filtering via SpiceDB** using `CheckPermission`

Even though retrieval is simple, the post-filter pattern mirrors how real RAG systems use SpiceDB alongside a vector database.

### ✔️ Assert permission-aware results  
The test checks that:

- **Emilia** sees `doc1` and `doc3`, but not `doc2`  
- **Beatrice** sees `doc2` and `doc3`, but not `doc1`  
- **Charlie** only sees `doc3`

This proves that permissions are enforced correctly even inside automated tests.

---

## 🧱 Project Structure

```
.
├── rag.go                 # Minimal RAG pipeline with SpiceDB post-filtering
├── rag_spicedb_test.go    # Main test using Testcontainers + SpiceDB
└── go.mod                 # Dependencies
```

No external vector DBs or LLMs are used here — the goal is to keep the demo lightweight and focused on **authorization testing**.

- For a self-guided workshop on fine-grained authorization using pre-filter and post-filter visit [this repo](https://github.com/authzed/workshops/tree/main/secure-rag-pipelines)
- To build a production-grade multi-tenant RAG pipeline, follow [this guide](https://authzed.com/blog/building-a-multi-tenant-rag-with-fine-grain-authorization-using-motia-and-spicedb)

---

## 📦 Requirements

- Go 1.21+  
- Docker Desktop

---

## ▶️ Running the Tests

```bash
go test -v
```

You should see:

- Testcontainers starting a SpiceDB container
- Schema being written
- Relationships being inserted
- Permission-aware RAG results being asserted
- Test passing 🎉

