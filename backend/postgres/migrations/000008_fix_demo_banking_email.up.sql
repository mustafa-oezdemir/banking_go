DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM users WHERE email = 'demo.baking@pehlione.com')
       AND NOT EXISTS (SELECT 1 FROM users WHERE email = 'demo.banking@pehlione.com') THEN
        UPDATE users
        SET email = 'demo.banking@pehlione.com'
        WHERE email = 'demo.baking@pehlione.com';
    END IF;
END;
$$;
