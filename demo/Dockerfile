FROM openjdk:21-jdk-slim

WORKDIR /app

COPY gradlew gradlew
COPY gradle gradle
COPY build.gradle settings.gradle ./
COPY src src

RUN chmod +x gradlew
RUN ./gradlew build -x test

EXPOSE 8080

CMD ["java", "-jar", "build/libs/demo-0.0.1-SNAPSHOT.jar"]