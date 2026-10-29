// Tiny test app for Nectar Apps: an HTTP server on $PORT (default 8080).
//   GET /        → greeting with the env var GREETING, Java version and hostname
//   GET /health  → "ok"
// Build: ./build.sh  (needs a JDK; compiled for Java 17 so the panel detects "Java 17")
import com.sun.net.httpserver.HttpServer;
import java.io.OutputStream;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;

public class Hello {
    public static void main(String[] args) throws Exception {
        int port = Integer.parseInt(System.getenv().getOrDefault("PORT", "8080"));
        HttpServer server = HttpServer.create(new InetSocketAddress(port), 0);
        server.createContext("/health", ex -> reply(ex, 200, "ok\n"));
        server.createContext("/", ex -> reply(ex, 200,
            System.getenv().getOrDefault("GREETING", "Hello") + " from Nectar Apps!\n"
            + "java:    " + System.getProperty("java.version") + "\n"
            + "host:    " + InetAddress.getLocalHost().getHostName() + "\n"
            + "release: " + System.getenv().getOrDefault("NECTAR_RELEASE", "?") + "\n"));
        server.start();
        System.out.println("listening on :" + port);
    }

    static void reply(com.sun.net.httpserver.HttpExchange ex, int code, String body) throws java.io.IOException {
        byte[] b = body.getBytes(StandardCharsets.UTF_8);
        ex.getResponseHeaders().set("Content-Type", "text/plain; charset=utf-8");
        ex.sendResponseHeaders(code, b.length);
        try (OutputStream os = ex.getResponseBody()) { os.write(b); }
    }
}
