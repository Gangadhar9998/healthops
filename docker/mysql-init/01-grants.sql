-- Grant healthmon user access to performance_schema for monitoring
GRANT SELECT ON performance_schema.* TO 'healthmon'@'%';
FLUSH PRIVILEGES;
