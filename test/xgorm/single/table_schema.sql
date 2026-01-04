-- PostgreSQL 表结构

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    email VARCHAR(100) UNIQUE NOT NULL,
    age INT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 插入测试数据
INSERT INTO users (name, email, age) VALUES
    ('张三', 'zhangsan@example.com', 25),
    ('李四', 'lisi@example.com', 30),
    ('王五', 'wangwu@example.com', 28),
    ('赵六', 'zhaoliu@example.com', 35),
    ('钱七', 'qianqi@example.com', 22);
