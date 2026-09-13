DROP TABLE IF EXISTS merchant_payment_intents;
DROP TABLE IF EXISTS merchants;

DELETE FROM accounts WHERE iban = 'DE89999999998000000000';
DELETE FROM users WHERE email = 'merchant.pehlione-ecommerce@invalid.local';
