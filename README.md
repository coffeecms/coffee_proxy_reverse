Here’s a detailed `README.md` for your reverse proxy management system in English: https://blog.lowlevelforest.com/

---

# Reverse Proxy Management System

## Introduction

The **Reverse Proxy Management System** is built with Go, designed to handle multiple domains and route traffic to different backend servers using individual configuration files. The system supports:
- Reverse proxy for multiple domains.
- Configuration through `.conf` files in the `list_domain` directory.
- Automatic reloading and updating when configuration files change.
- Rate limiting, IP whitelisting/blacklisting, request size limits, and optional SSL/TLS encryption.

## Prerequisites

- Go 1.16 or later
- A Unix-based operating system (Linux, macOS) or Windows

## Installation

1. **Clone the Repository:**

   ```bash
   git clone https://github.com/coffeecms/coffee_proxy_reverse.git
   cd coffee_proxy_reverse
   ```

2. **Build the Project:**

   ```bash
   go build -o coffee_proxy_reverse
   ```

3. **Configuration:**
   - Create a `system.conf` file in the project root directory.
   - Create a `list_domain` directory to store domain configuration files.

## Configuration

### `system.conf`

The `system.conf` file contains global settings for rate limiting, timeouts, request size limits, IP filtering, and SSL/TLS settings. Here’s an example configuration:

```ini
[rate_limiting]
requests_per_second = 1
burst_limit = 5

[timeouts]
read_timeout = 5
write_timeout = 10
idle_timeout = 30

[request_limits]
max_request_size = 1048576  # 1MB in bytes

[ssl]
enabled = true
cert_file = "server.crt"
key_file = "server.key"

[whitelist]
ips = "192.168.1.1,10.0.0.1"

[blacklist]
ips = "203.0.113.10"
```

### Domain Configuration Files

Each domain should have a separate `.conf` file in the `list_domain` directory. The file should be named `<domain>.conf` and contain the following:

```ini
[proxy]
backend_url = "http://backend_server_ip:port"
```

Replace `backend_server_ip:port` with the actual address and port of the backend server.

## Running the Server

1. **Start the Server:**

   ```bash
   ./coffee_proxy_reverse
   ```

2. **Accessing the Server:**
   - If SSL/TLS is enabled, access the server using HTTPS: `https://localhost:8080`
   - Otherwise, use HTTP: `http://localhost:8080`

## Adding/Removing Domains

### Adding a Domain

1. **Create a Configuration File:**
   - Create a new `.conf` file in the `list_domain` directory.
   - Name the file according to the domain you are configuring (e.g., `example.com.conf`).

2. **Edit the File:**
   - Add the backend URL as described above.

3. **Reload Configuration:**
   - The system automatically detects changes in the `list_domain` directory and reloads the configuration.

### Removing a Domain

1. **Delete the Configuration File:**
   - Remove the `.conf` file corresponding to the domain from the `list_domain` directory.

2. **Reload Configuration:**
   - The system will automatically detect the removal and update its configuration.

## Monitoring and Logs

- Logs and status messages will be output to the console. Review these logs to monitor the operation of the reverse proxy and diagnose any issues.

## Contributing

Feel free to contribute to this project by opening issues or submitting pull requests. Ensure your contributions adhere to the coding style and include tests.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Contact

For any questions or support, please contact [lowlevelforest@gmail.com](mailto:lowlevelforest@gmail.com).
