"""
Mock image server for testing firmware URL resolution and HTTP transfers.
Serves dummy firmware files matching production patterns.
"""

import http.server
import socketserver
import threading
import tempfile
import os
from pathlib import Path


class MockImageHandler(http.server.SimpleHTTPRequestHandler):
    """HTTP handler that serves firmware files from a temporary directory."""
    
    def __init__(self, *args, **kwargs):
        self.temp_dir = kwargs.pop('temp_dir')
        super().__init__(*args, directory=self.temp_dir, **kwargs)
    
    def log_message(self, format, *args):
        """Suppress default logging to avoid cluttering test output."""
        pass


class MockImageServer:
    """
    Mock HTTP server that serves firmware images for testing.
    
    Serves files at /images/ matching production patterns:
    - sonic-mellanox-20241212.01.bin
    - sonic-aboot-broadcom-20250510.18.swi
    - sonic-cisco-20241201.05.bin
    """
    
    def __init__(self, port=8080):
        self.port = port
        self.server = None
        self.thread = None
        self.temp_dir = None
        self.base_path = "images"  # Keep internal path structure private
    
    def setup_files(self):
        """Create temporary directory and dummy firmware files."""
        self.temp_dir = tempfile.mkdtemp()
        
        # Create directory structure
        acs_dir = Path(self.temp_dir) / self.base_path
        acs_dir.mkdir(parents=True, exist_ok=True)
        
        # Create dummy firmware files matching production patterns
        firmware_files = [
            # Mellanox files
            "sonic-mellanox-20241212.01.bin",
            "sonic-mellanox-20241215.02.bin",
            "sonic-mellanox-202505.01.bin",  # Match test data
            
            # Broadcom Aboot files  
            "sonic-aboot-broadcom-20250510.18.swi",
            "sonic-aboot-broadcom-20241201.05.swi",
            
            # Cisco files
            "sonic-cisco-20241201.05.bin",
            "sonic-cisco-20241210.10.bin",
            
            # Arista files
            "sonic-arista-20241205.03.bin",
        ]
        
        for filename in firmware_files:
            file_path = acs_dir / filename
            # Create dummy content - small but realistic
            dummy_content = f"DUMMY_FIRMWARE_FILE:{filename}\n" * 100
            file_path.write_text(dummy_content)
        
        print(f"Created {len(firmware_files)} dummy firmware files in {acs_dir}")
    
    def start(self):
        """Start the HTTP server in a background thread."""
        if self.server:
            return  # Already started
        
        self.setup_files()
        
        # Create handler with temp directory
        def handler(*args, **kwargs):
            return MockImageHandler(*args, temp_dir=self.temp_dir, **kwargs)
        
        self.server = socketserver.TCPServer(("", self.port), handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        
        print(f"Mock image server started on port {self.port}")
        print(f"Serving files from: http://localhost:{self.port}/{self.base_path}/")
    
    def stop(self):
        """Stop the HTTP server and cleanup temporary files."""
        if self.server:
            self.server.shutdown()
            self.server.server_close()
            self.thread.join(timeout=5)
            self.server = None
            self.thread = None
        
        if self.temp_dir:
            import shutil
            shutil.rmtree(self.temp_dir, ignore_errors=True)
            self.temp_dir = None
        
        print(f"Mock image server stopped")
    
    def get_url(self, filename):
        """Get the full URL for a firmware file."""
        return f"http://localhost:{self.port}/{self.base_path}/{filename}"
    
    def list_files(self):
        """List all available firmware files."""
        if not self.temp_dir:
            return []
        
        acs_dir = Path(self.temp_dir) / self.base_path
        if not acs_dir.exists():
            return []
        
        return [f.name for f in acs_dir.iterdir() if f.is_file()]


if __name__ == "__main__":
    # Test the server
    import time
    import requests
    
    server = MockImageServer(8080)
    try:
        server.start()
        time.sleep(1)  # Let server start
        
        # Test fetching a file
        url = server.get_url("sonic-mellanox-20241212.01.bin")
        print(f"Testing URL: {url}")
        
        response = requests.get(url)
        if response.status_code == 200:
            print(f"✅ Successfully fetched {len(response.content)} bytes")
        else:
            print(f"❌ Failed to fetch: {response.status_code}")
        
        print(f"Available files: {server.list_files()}")
        
    except Exception as e:
        print(f"Error: {e}")
    finally:
        server.stop()