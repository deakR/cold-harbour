package com.coldharbor.controlplane;

import com.coldharbor.controlplane.config.TestRedisConfig;
import org.junit.jupiter.api.Test;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.context.annotation.Import;
import org.springframework.test.context.ActiveProfiles;

@SpringBootTest
@ActiveProfiles("test")
@Import(TestRedisConfig.class)
class ControlPlaneApplicationTests {

    @Test
    void contextLoads() {
    }
}
