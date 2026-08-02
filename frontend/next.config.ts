import type { NextConfig } from "next";

const contentSecurityPolicyDirectives = [
  "default-src 'self'",
  "base-uri 'self'",
  "form-action 'self'",
  "frame-ancestors 'none'",
  "object-src 'none'",
  "script-src 'self' 'unsafe-inline'",
	"style-src 'self' 'unsafe-inline'",
	"font-src 'self' data:",
  "img-src 'self' data:",
  "connect-src 'self'",
  "worker-src 'self' blob:",
];
if (process.env.NODE_ENV === "production") {
  contentSecurityPolicyDirectives.push("upgrade-insecure-requests");
}
const contentSecurityPolicy = contentSecurityPolicyDirectives.join("; ");

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
  headers: async () => [
    {
      source: "/(.*)",
      headers: [
        { key: "Content-Security-Policy", value: contentSecurityPolicy },
        { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
        { key: "X-Content-Type-Options", value: "nosniff" },
        { key: "X-Frame-Options", value: "DENY" },
        {
          key: "Permissions-Policy",
          value: "camera=(), microphone=(), geolocation=(), payment=()",
        },
        {
          key: "Strict-Transport-Security",
          value: "max-age=31536000; includeSubDomains",
        },
      ],
    },
  ],
  rewrites: async () => {
    const apiBaseUrl =
      process.env.BACKEND_API_URL ||
      process.env.NEXT_PUBLIC_API_BASE_URL ||
      "http://localhost:8080";
    return {
      beforeFiles: [
        // Proxy authentication endpoints
        {
          source: "/register",
          destination: `${apiBaseUrl}/register`,
        },
        {
          source: "/login",
          destination: `${apiBaseUrl}/login`,
        },
        {
          source: "/logout",
          destination: `${apiBaseUrl}/logout`,
        },
        {
          source: "/session",
          destination: `${apiBaseUrl}/session`,
        },
        // Proxy accounts endpoints (both /accounts and /accounts/*)
        {
          source: "/accounts",
          destination: `${apiBaseUrl}/accounts`,
        },
        {
          source: "/accounts/:path*",
          destination: `${apiBaseUrl}/accounts/:path*`,
        },
        // Proxy transfer endpoints
        {
          source: "/transfers",
          destination: `${apiBaseUrl}/transfers`,
        },
        // Proxy transaction endpoints
        {
          source: "/transactions/:path*",
          destination: `${apiBaseUrl}/transactions/:path*`,
        },
		{
			source: "/payments/:path*",
			destination: `${apiBaseUrl}/payments/:path*`,
		},
		{
			source: "/payments",
			destination: `${apiBaseUrl}/payments`,
		},
		{
			source: "/payees/:path*",
			destination: `${apiBaseUrl}/payees/:path*`,
		},
		{
			source: "/standing-orders/:path*",
			destination: `${apiBaseUrl}/standing-orders/:path*`,
		},
		{
			source: "/standing-orders",
			destination: `${apiBaseUrl}/standing-orders`,
		},
		{
			source: "/beneficiaries",
			destination: `${apiBaseUrl}/beneficiaries`,
		},
		{
			source: "/events",
			destination: `${apiBaseUrl}/events`,
		},
        // Proxy swagger docs to Go backend
        {
          source: "/swagger/:path*",
          destination: `${apiBaseUrl}/swagger/:path*`,
        },
        // Proxy health endpoint
        {
          source: "/health",
          destination: `${apiBaseUrl}/health`,
        },
      ],
    };
  },
};

export default nextConfig;
