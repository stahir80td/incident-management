"""
Incident Knowledge Base Ingestion Pipeline
Reads RCA markdown files, generates embeddings using Gemini, and uploads to Qdrant
"""

import os
import glob
import re
from typing import List, Dict, Any
import google.generativeai as genai
from qdrant_client import QdrantClient
from qdrant_client.models import Distance, VectorParams, PointStruct
import time
from dotenv import load_dotenv

# ============================================================================
# CONFIGURATION
# ============================================================================

# Load environment variables from .env file
load_dotenv()

# Get API Keys from environment
GEMINI_API_KEY = os.getenv("GEMINI_API_KEY")
QDRANT_URL = os.getenv("QDRANT_URL")
QDRANT_API_KEY = os.getenv("QDRANT_API_KEY")

# Validate required environment variables
if not GEMINI_API_KEY:
    raise ValueError("GEMINI_API_KEY not found in environment variables. Please check your .env file.")
if not QDRANT_URL:
    raise ValueError("QDRANT_URL not found in environment variables. Please check your .env file.")
if not QDRANT_API_KEY:
    raise ValueError("QDRANT_API_KEY not found in environment variables. Please check your .env file.")

# Paths
INCIDENTS_DIR = os.getenv("INCIDENTS_DIR", r"C:\agents\incident-management\incidents")
COLLECTION_NAME = os.getenv("COLLECTION_NAME", "incident-knowledge-base")

# Gemini embedding model
EMBEDDING_MODEL = "models/gemini-embedding-001"
EMBEDDING_DIMENSION = 3072  

# ============================================================================
# HELPER FUNCTIONS
# ============================================================================

def parse_incident_file(filepath: str) -> Dict[str, Any]:
    """
    Parse a markdown incident file and extract metadata + content sections
    """
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()
    
    # Extract YAML frontmatter
    metadata = {}
    frontmatter_match = re.match(r'^---\s*\n(.*?)\n---\s*\n', content, re.DOTALL)
    if frontmatter_match:
        frontmatter = frontmatter_match.group(1)
        for line in frontmatter.split('\n'):
            if ':' in line:
                key, value = line.split(':', 1)
                metadata[key.strip()] = value.strip()
        
        # Remove frontmatter from content
        content = content[frontmatter_match.end():]
    
    # Extract sections using markdown headers
    sections = {}
    current_section = "header"
    current_content = []
    
    for line in content.split('\n'):
        if line.startswith('# '):
            # Main title
            if current_content:
                sections[current_section] = '\n'.join(current_content).strip()
            current_section = "title"
            current_content = [line[2:].strip()]
        elif line.startswith('## '):
            # Section header
            if current_content:
                sections[current_section] = '\n'.join(current_content).strip()
            current_section = line[3:].strip().lower().replace(' ', '_')
            current_content = []
        else:
            current_content.append(line)
    
    # Add last section
    if current_content:
        sections[current_section] = '\n'.join(current_content).strip()
    
    return {
        'metadata': metadata,
        'sections': sections,
        'filepath': filepath
    }


def create_chunks(incident: Dict[str, Any]) -> List[Dict[str, Any]]:
    """
    Create intelligent chunks from incident data
    Each section becomes its own chunk for better semantic retrieval
    """
    chunks = []
    metadata = incident['metadata']
    sections = incident['sections']
    
    # Important sections for retrieval
    section_priority = ['summary', 'root_cause', 'resolution', 'prevention', 'impact', 'timeline']
    
    for section_name in section_priority:
        if section_name in sections and sections[section_name]:
            chunk = {
                'text': sections[section_name],
                'metadata': {
                    'incident_id': metadata.get('incident_id', 'unknown'),
                    'severity': metadata.get('severity', 'unknown'),
                    'service': metadata.get('service', 'unknown'),
                    'date': metadata.get('date', 'unknown'),
                    'section': section_name,
                    'filename': os.path.basename(incident['filepath'])
                }
            }
            chunks.append(chunk)
    
    return chunks


def generate_embedding(text: str, task_type: str = "RETRIEVAL_DOCUMENT") -> List[float]:
    """
    Generate embedding for text using Gemini API
    task_type: RETRIEVAL_DOCUMENT for documents, RETRIEVAL_QUERY for queries
    """
    try:
        result = genai.embed_content(
            model=EMBEDDING_MODEL,
            content=text,
            task_type=task_type,
            output_dimensionality=EMBEDDING_DIMENSION
        )
        return result['embedding']
    except Exception as e:
        print(f"Error generating embedding: {e}")
        raise


def batch_generate_embeddings(chunks: List[Dict[str, Any]], batch_size: int = 5) -> List[Dict[str, Any]]:
    """
    Generate embeddings for all chunks with rate limiting
    Gemini free tier: 1,500 requests/day, so we add delays
    """
    print(f"Generating embeddings for {len(chunks)} chunks...")
    
    for i, chunk in enumerate(chunks):
        try:
            # Generate embedding
            embedding = generate_embedding(chunk['text'])
            chunk['embedding'] = embedding
            
            # Progress update
            if (i + 1) % 10 == 0:
                print(f"  Processed {i + 1}/{len(chunks)} chunks")
            
            # Rate limiting: ~1 request per second to be safe
            time.sleep(1)
            
        except Exception as e:
            print(f"  Failed on chunk {i}: {e}")
            raise
    
    print(f"✓ Generated {len(chunks)} embeddings")
    return chunks


def upload_to_qdrant(chunks: List[Dict[str, Any]]) -> None:
    """
    Upload embedded chunks to Qdrant vector database
    """
    print(f"\nConnecting to Qdrant at {QDRANT_URL}...")
    
    # Initialize Qdrant client
    client = QdrantClient(
        url=QDRANT_URL,
        api_key=QDRANT_API_KEY,
    )
    
    # Create collection if it doesn't exist
    try:
        client.get_collection(COLLECTION_NAME)
        print(f"✓ Collection '{COLLECTION_NAME}' already exists")
        
        # Delete and recreate for fresh start
        user_input = input("Delete existing collection and start fresh? (y/n): ")
        if user_input.lower() == 'y':
            client.delete_collection(COLLECTION_NAME)
            print(f"✓ Deleted existing collection")
            raise Exception("Recreate collection")
    except:
        # Create new collection
        client.create_collection(
            collection_name=COLLECTION_NAME,
            vectors_config=VectorParams(
                size=EMBEDDING_DIMENSION,
                distance=Distance.COSINE
            )
        )
        print(f"✓ Created new collection '{COLLECTION_NAME}'")
    
    # Prepare points for upload
    points = []
    for i, chunk in enumerate(chunks):
        point = PointStruct(
            id=i,
            vector=chunk['embedding'],
            payload={
                'text': chunk['text'],
                'incident_id': chunk['metadata']['incident_id'],
                'severity': chunk['metadata']['severity'],
                'service': chunk['metadata']['service'],
                'date': chunk['metadata']['date'],
                'section': chunk['metadata']['section'],
                'filename': chunk['metadata']['filename']
            }
        )
        points.append(point)
    
    # Upload in batches
    batch_size = 100
    for i in range(0, len(points), batch_size):
        batch = points[i:i + batch_size]
        client.upsert(
            collection_name=COLLECTION_NAME,
            points=batch
        )
        print(f"  Uploaded batch {i // batch_size + 1} ({len(batch)} points)")
    
    print(f"✓ Uploaded {len(points)} vectors to Qdrant")
    
    # Verify upload
    collection_info = client.get_collection(COLLECTION_NAME)
    print(f"\nCollection Info:")
    print(f"  - Vectors count: {collection_info.vectors_count}")
    print(f"  - Points count: {collection_info.points_count}")


def test_search(query: str = "database connection pool exhausted") -> None:
    """
    Test semantic search on the uploaded vectors
    """
    print(f"\n{'='*60}")
    print(f"Testing Search: '{query}'")
    print(f"{'='*60}")
    
    # Connect to Qdrant
    client = QdrantClient(
        url=QDRANT_URL,
        api_key=QDRANT_API_KEY,
    )
    
    # Generate query embedding
    print("Generating query embedding...")
    query_embedding = generate_embedding(query, task_type="RETRIEVAL_QUERY")
    
    # Search
    print("Searching...")
    results = client.search(
        collection_name=COLLECTION_NAME,
        query_vector=query_embedding,
        limit=3
    )
    
    # Display results
    print(f"\nTop 3 Results:\n")
    for i, result in enumerate(results, 1):
        print(f"{i}. [Score: {result.score:.4f}] {result.payload['incident_id']} - {result.payload['section']}")
        print(f"   Service: {result.payload['service']}")
        print(f"   Severity: {result.payload['severity']}")
        print(f"   Text preview: {result.payload['text'][:200]}...")
        print()


# ============================================================================
# MAIN PIPELINE
# ============================================================================

def main():
    """
    Main ingestion pipeline
    """
    print("="*60)
    print("INCIDENT KNOWLEDGE BASE INGESTION PIPELINE")
    print("="*60)
    
    # Configure Gemini
    genai.configure(api_key=GEMINI_API_KEY)
    
    # Step 1: Load all incident files
    print(f"\n[1/4] Loading incident files from: {INCIDENTS_DIR}")
    incident_files = glob.glob(os.path.join(INCIDENTS_DIR, "*.md"))
    print(f"✓ Found {len(incident_files)} incident files")
    
    if len(incident_files) == 0:
        print("ERROR: No incident files found!")
        print(f"Please check that files exist in: {INCIDENTS_DIR}")
        return
    
    # Step 2: Parse incidents and create chunks
    print(f"\n[2/4] Parsing incidents and creating chunks...")
    all_chunks = []
    for filepath in incident_files:
        try:
            incident = parse_incident_file(filepath)
            chunks = create_chunks(incident)
            all_chunks.extend(chunks)
        except Exception as e:
            print(f"  Error parsing {filepath}: {e}")
    
    print(f"✓ Created {len(all_chunks)} chunks from {len(incident_files)} incidents")
    print(f"  (Average: {len(all_chunks) // len(incident_files)} chunks per incident)")
    
    # Step 3: Generate embeddings
    print(f"\n[3/4] Generating embeddings using Gemini ({EMBEDDING_MODEL})...")
    print(f"  Dimension: {EMBEDDING_DIMENSION}")
    print(f"  This will take ~{len(all_chunks)} seconds (rate limiting)...")
    
    all_chunks = batch_generate_embeddings(all_chunks)
    
    # Step 4: Upload to Qdrant
    print(f"\n[4/4] Uploading to Qdrant...")
    upload_to_qdrant(all_chunks)
    
    # Test the search
    test_search()
    
    print(f"\n{'='*60}")
    print("✓ INGESTION COMPLETE!")
    print(f"{'='*60}")
    print(f"\nYour knowledge base is ready with {len(all_chunks)} searchable chunks!")


if __name__ == "__main__":
    main()