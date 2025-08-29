-- Cria o banco de dados.
-- O comando IF NOT EXISTS evita um erro caso o banco de dados já exista.
CREATE DATABASE IF NOT EXISTS loa_db;

-- Cria um novo usuário e define a senha.
-- A cláusula 'localhost' garante que o usuário só possa se conectar do próprio servidor.
-- Para permitir conexões de qualquer host, use '%' em vez de 'localhost'.
CREATE USER 'loa_user'@'sig-srv-01.subnet04162109.vcn04162109.oraclevcn.com' IDENTIFIED BY 'password';

-- Concede privilégios ao usuário para o banco de dados específico.
-- A cláusula 'privilegios' indica quais permissões o usuário terá.
--
-- PRIVILÉGIOS:
-- CREATE: Permite criar tabelas ou índices.
-- ALTER: Permite alterar tabelas.
-- INDEX: Permite criar e remover índices.
-- DROP: Permite remover tabelas.
-- INSERT, UPDATE, DELETE, SELECT: Permitem a manipulação de dados.
--
-- O privilégio 'GRANT OPTION' permite que o usuário conceda privilégios a outros usuários.
--
GRANT CREATE, ALTER, INDEX, DROP, INSERT, UPDATE, DELETE, REFERENCES, SELECT ON loa_db.* TO 'loa_user'@'sig-srv-01.subnet04162109.vcn04162109.oraclevcn.com';

USE loa_db;

--DROP TABLE tb_history;
-- Create the main history table
CREATE TABLE IF NOT EXISTS tb_history (
    id VARCHAR(255) PRIMARY KEY,
    event_type VARCHAR(50) NOT NULL,
    original_message_id VARCHAR(255),
    account VARCHAR(255) NOT NULL,
    sender_number VARCHAR(255),
    sender_name VARCHAR(255),
    recipient VARCHAR(255),
    message_content TEXT,
    attachments_exist BOOLEAN, -- MySQL maps BOOLEAN to TINYINT(1)
    timestamp_service BIGINT NOT NULL,
    timestamp_signal BIGINT,
    raw_payload JSON NOT NULL, -- Requires MySQL 5.7.8+
    processed_by_server VARCHAR(255) NOT NULL,
    logged_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_history_sender (sender_number),
    INDEX idx_history_recipient (recipient)
);

--DROP TABLE tb_history_attachments;
-- Create the attachments history table
CREATE TABLE IF NOT EXISTS tb_history_attachments (
    id VARCHAR(255) PRIMARY KEY,
    history_id VARCHAR(255) NOT NULL,
    content_type VARCHAR(255),
    filename VARCHAR(255),
    size INT,
    width INT,
    height INT,
    caption TEXT,
    upload_timestamp BIGINT,
    logged_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,    
    -- Define the foreign key constraint
    FOREIGN KEY (history_id) REFERENCES tb_history(id) ON DELETE CASCADE,
    INDEX idx_history_attachments_history_id (history_id)
);