# syntax=docker/dockerfile:1
# Expects a prebuilt jar: run `make build` (or ./mvnw -q -DskipTests package) first.
FROM eclipse-temurin:21-jre-alpine
WORKDIR /app
COPY target/control-plane-*.jar /app/app.jar
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=10 \
  CMD wget -qO- http://localhost:8080/actuator/health | grep -q '"status":"UP"'
ENTRYPOINT ["java", "-jar", "/app/app.jar"]
