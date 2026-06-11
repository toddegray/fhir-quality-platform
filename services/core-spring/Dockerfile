# syntax=docker/dockerfile:1.7

# --- build stage ----------------------------------------------------------
FROM maven:3.9-eclipse-temurin-21 AS build
WORKDIR /src

COPY pom.xml ./
RUN mvn -B -q dependency:go-offline

COPY src ./src
RUN mvn -B -q -DskipTests package

# --- runtime stage --------------------------------------------------------
FROM eclipse-temurin:21-jre-alpine
RUN adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /src/target/core-spring-0.1.0.jar /app/app.jar
USER app
EXPOSE 8080
ENTRYPOINT ["java", "-jar", "/app/app.jar"]
