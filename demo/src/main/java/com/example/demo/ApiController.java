package com.example.demo;

import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.reactive.function.client.WebClient;
import reactor.core.publisher.Mono;

@RestController
public class ApiController {

    private final WebClient webClient;

    public ApiController(WebClient.Builder webClientBuilder) {
        this.webClient = webClientBuilder.baseUrl("http://envoy:8081").build();
    }

    @GetMapping("/api/data")
    public Mono<String> getData() {
        return webClient.get()
                .uri("/external/data")
                .retrieve()
                .bodyToMono(String.class);
    }

    @GetMapping("/health")
    public String health() {
        return "OK";
    }
}