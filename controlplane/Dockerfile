FROM maven:3.9-eclipse-temurin-21 AS build
WORKDIR /src
COPY controlplane/pom.xml controlplane/pom.xml
COPY controlplane/src controlplane/src
COPY internal/runner/types.txt internal/runner/types.txt
WORKDIR /src/controlplane
RUN mvn -q -DskipTests package

FROM eclipse-temurin:21-jre
COPY --from=build /src/controlplane/target/controlplane-0.0.1-SNAPSHOT.jar /app.jar
ENTRYPOINT ["java", "-jar", "/app.jar"]
