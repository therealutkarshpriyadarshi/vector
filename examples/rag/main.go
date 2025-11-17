package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/therealutkarshpriyadarshi/vector/pkg/api/grpc/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	serverAddr = "localhost:50051"
	namespace  = "rag-demo"
)

// Document represents a knowledge base document
type Document struct {
	ID      string
	Title   string
	Content string
}

// Simple embedding function (in production, use OpenAI, Sentence-BERT, etc.)
// This creates a simple deterministic embedding based on word hashing
func simpleEmbedding(text string, dimensions int) []float32 {
	words := strings.Fields(strings.ToLower(text))
	embedding := make([]float32, dimensions)

	// Simple word hashing approach
	for _, word := range words {
		hash := hashString(word)
		for i := 0; i < dimensions; i++ {
			// Use word hash to generate pseudo-random but deterministic values
			seed := hash + uint32(i)
			val := float32(seed%1000) / 1000.0
			embedding[i] += val
		}
	}

	// Normalize
	var norm float32
	for _, v := range embedding {
		norm += v * v
	}
	norm = float32(math.Sqrt(float64(norm)))

	if norm > 0 {
		for i := range embedding {
			embedding[i] /= norm
		}
	}

	return embedding
}

// Simple string hashing function
func hashString(s string) uint32 {
	var hash uint32 = 5381
	for _, c := range s {
		hash = ((hash << 5) + hash) + uint32(c)
	}
	return hash
}

func main() {
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║  RAG (Retrieval-Augmented Generation) Demo               ║")
	fmt.Println("║  Vector Database + Semantic Search                       ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Connect to vector database
	fmt.Printf("Connecting to vector database at %s...\n", serverAddr)
	client, conn, err := connectToServer()
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	// Check health
	healthResp, err := client.HealthCheck(ctx, &proto.HealthCheckRequest{})
	if err != nil {
		log.Fatalf("Health check failed: %v", err)
	}
	fmt.Printf("✓ Connected (status: %s, version: %s)\n\n", healthResp.Status, healthResp.Version)

	// Create knowledge base
	docs := getKnowledgeBase()
	fmt.Printf("Indexing %d documents into vector database...\n", len(docs))

	// Index documents
	indexed := 0
	for _, doc := range docs {
		// Create embedding (in production, use OpenAI API or similar)
		embedding := simpleEmbedding(doc.Title+" "+doc.Content, 128)

		// Insert into vector database
		req := &proto.InsertRequest{
			Namespace: namespace,
			Vector:    embedding,
			Metadata: map[string]string{
				"doc_id": doc.ID,
				"title":  doc.Title,
			},
			Text: &doc.Content,
		}

		_, err := client.Insert(ctx, req)
		if err != nil {
			log.Printf("Warning: failed to index document %s: %v", doc.ID, err)
			continue
		}

		indexed++
	}

	fmt.Printf("✓ Indexed %d documents\n\n", indexed)

	// Wait for indexing to complete
	time.Sleep(200 * time.Millisecond)

	// Interactive Q&A loop
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("Ask questions about the knowledge base (or 'quit' to exit)")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("Question: ")
		if !scanner.Scan() {
			break
		}

		question := strings.TrimSpace(scanner.Text())
		if question == "" {
			continue
		}
		if strings.ToLower(question) == "quit" || strings.ToLower(question) == "exit" {
			break
		}

		// Retrieve relevant documents
		fmt.Println("\nRetrieving relevant documents...")
		retrievedDocs := retrieveDocuments(ctx, client, question, 3)

		if len(retrievedDocs) == 0 {
			fmt.Println("No relevant documents found.\n")
			continue
		}

		// Display retrieved documents
		fmt.Printf("\nFound %d relevant documents:\n\n", len(retrievedDocs))
		for i, result := range retrievedDocs {
			title := result.Metadata["title"]
			fmt.Printf("%d. %s (relevance: %.2f%%)\n", i+1, title, (1-result.Distance)*100)
			if result.Text != nil {
				content := truncateString(*result.Text, 200)
				fmt.Printf("   %s\n\n", content)
			}
		}

		// In a real RAG system, you would now:
		// 1. Combine retrieved documents into context
		// 2. Send to LLM (GPT-4, Claude, etc.) with the question
		// 3. Return the generated answer
		//
		// For this demo, we just show the retrieval part
		fmt.Println("─────────────────────────────────────────────────────────────")
		fmt.Println("Note: In a full RAG system, this context would be sent to an")
		fmt.Println("LLM (like GPT-4) to generate a comprehensive answer.")
		fmt.Println("─────────────────────────────────────────────────────────────")
		fmt.Println()
	}

	fmt.Println("\n✓ Demo complete. Thank you!")
}

func connectToServer() (proto.VectorDBClient, *grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, nil, err
	}

	return proto.NewVectorDBClient(conn), conn, nil
}

func retrieveDocuments(ctx context.Context, client proto.VectorDBClient, query string, k int) []*proto.SearchResult {
	// Create query embedding
	queryEmbedding := simpleEmbedding(query, 128)

	// Hybrid search combining vector similarity and text search
	req := &proto.HybridSearchRequest{
		Namespace:   namespace,
		QueryVector: queryEmbedding,
		QueryText:   query,
		K:           int32(k),
		EfSearch:    50,
	}

	resp, err := client.HybridSearch(ctx, req)
	if err != nil {
		log.Printf("Search failed: %v", err)
		return nil
	}

	return resp.Results
}

func getKnowledgeBase() []Document {
	return []Document{
		{
			ID:    "doc1",
			Title: "What is HNSW?",
			Content: "HNSW (Hierarchical Navigable Small World) is a state-of-the-art algorithm for " +
				"approximate nearest neighbor search. It builds a multi-layer graph structure that " +
				"allows for fast and accurate similarity search in high-dimensional spaces. HNSW is " +
				"used by major vector databases like Weaviate, Qdrant, and Milvus.",
		},
		{
			ID:    "doc2",
			Title: "Vector Databases Explained",
			Content: "Vector databases are specialized database systems designed to store and search " +
				"vector embeddings efficiently. They are critical for modern AI applications including " +
				"semantic search, recommendation systems, and RAG (Retrieval-Augmented Generation). " +
				"Unlike traditional databases that use exact matching, vector databases find similar " +
				"items based on mathematical distance metrics.",
		},
		{
			ID:    "doc3",
			Title: "What is RAG?",
			Content: "RAG (Retrieval-Augmented Generation) is a technique that enhances large language " +
				"models by providing them with relevant context retrieved from a knowledge base. The " +
				"process involves: 1) Converting documents to vector embeddings, 2) Storing them in a " +
				"vector database, 3) Retrieving relevant documents for a given query, and 4) Passing " +
				"the retrieved context to an LLM to generate an informed response.",
		},
		{
			ID:    "doc4",
			Title: "Hybrid Search Benefits",
			Content: "Hybrid search combines vector similarity search with traditional keyword search " +
				"(like BM25). This approach provides the best of both worlds: semantic understanding " +
				"from vector search and exact term matching from keyword search. Studies show that " +
				"hybrid search typically outperforms either method alone, especially for queries that " +
				"contain specific terms or proper nouns.",
		},
		{
			ID:    "doc5",
			Title: "Cosine Similarity vs Euclidean Distance",
			Content: "Cosine similarity and Euclidean distance are two common distance metrics for " +
				"vector search. Cosine similarity measures the angle between vectors and is " +
				"scale-invariant, making it ideal for text embeddings. Euclidean distance measures " +
				"the straight-line distance between vectors and considers magnitude. The choice " +
				"depends on your data and use case.",
		},
		{
			ID:    "doc6",
			Title: "Scaling Vector Databases",
			Content: "Scaling vector databases requires several techniques: 1) Quantization to reduce " +
				"memory usage, 2) Sharding to distribute data across machines, 3) Caching for frequent " +
				"queries, 4) SIMD optimizations for faster distance calculations, and 5) Approximate " +
				"algorithms like HNSW that trade perfect accuracy for speed. Production systems often " +
				"combine multiple techniques.",
		},
		{
			ID:    "doc7",
			Title: "Embeddings in Machine Learning",
			Content: "Embeddings are dense vector representations of data (text, images, audio) in a " +
				"continuous space. They capture semantic meaning, allowing similar items to have " +
				"similar vectors. Modern embedding models like OpenAI's text-embedding-3, Google's " +
				"Universal Sentence Encoder, and Sentence-BERT can create high-quality embeddings " +
				"for various types of content.",
		},
		{
			ID:    "doc8",
			Title: "Multi-Tenancy in Databases",
			Content: "Multi-tenancy allows a single database instance to serve multiple customers " +
				"(tenants) while maintaining data isolation. This is achieved through namespaces or " +
				"separate schemas. Multi-tenancy reduces operational costs and complexity compared to " +
				"running separate database instances for each customer. Proper isolation is critical " +
				"for security and compliance.",
		},
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
