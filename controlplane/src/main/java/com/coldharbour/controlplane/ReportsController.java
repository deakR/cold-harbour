package com.coldharbour.controlplane;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.UUID;

import org.springframework.http.HttpHeaders;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import jakarta.servlet.http.HttpServletRequest;

@RestController
public class ReportsController {

	private final JdbcTemplate jdbc;

	public ReportsController(JdbcTemplate jdbc) {
		this.jdbc = jdbc;
	}

	@GetMapping("/reports/compliance")
	public ResponseEntity<String> compliance(
			@RequestParam String from,
			@RequestParam String to,
			HttpServletRequest req) {
		UUID tenantId = (UUID) req.getAttribute(ApiKeyFilter.ATTR_TENANT);
		Instant fromAt = Instant.parse(from);
		Instant toAt = Instant.parse(to);
		List<Map<String, Object>> accepts = jdbc.queryForList("""
				SELECT redis_job_id, accepted_at
				FROM job_accepts
				WHERE tenant_id = ? AND accepted_at >= ? AND accepted_at < ?
				ORDER BY accepted_at
				""", tenantId, java.sql.Timestamp.from(fromAt), java.sql.Timestamp.from(toAt));
		StringBuilder csv = new StringBuilder();
		csv.append("dispatch_time,completion_time,final_state,checksum,signature_status,purge_timestamp\n");
		for (Map<String, Object> accept : accepts) {
			String redisID = (String) accept.get("redis_job_id");
			UUID durable = DurableID.forRedisJob(redisID);
			List<Map<String, Object>> jobs = jdbc.queryForList("""
					SELECT completed_at, final_state, output_checksum, output_signature
					FROM jobs WHERE id = ? AND tenant_id = ?
					""", durable, tenantId);
			List<Map<String, Object>> receipts = jdbc.queryForList(
					"SELECT purged_at FROM purge_receipts WHERE job_id = ?", durable);
			String completed = "";
			String state = "";
			String checksum = "";
			String signatureStatus = "absent";
			if (!jobs.isEmpty()) {
				Map<String, Object> job = jobs.get(0);
				completed = String.valueOf(job.get("completed_at"));
				state = String.valueOf(job.get("final_state"));
				Object sum = job.get("output_checksum");
				checksum = sum == null ? "" : sum.toString();
				if (job.get("output_signature") != null) {
					signatureStatus = "present";
				}
			}
			String purged = "";
			if (!receipts.isEmpty() && receipts.get(0).get("purged_at") != null) {
				purged = String.valueOf(receipts.get(0).get("purged_at"));
			}
			csv.append(accept.get("accepted_at")).append(',')
					.append(completed).append(',')
					.append(state).append(',')
					.append(checksum).append(',')
					.append(signatureStatus).append(',')
					.append(purged).append('\n');
		}
		return ResponseEntity.ok()
				.header(HttpHeaders.CONTENT_TYPE, "text/csv")
				.contentType(MediaType.parseMediaType("text/csv"))
				.body(csv.toString());
	}
}
