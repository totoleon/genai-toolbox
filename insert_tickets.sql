-- Insert 20 tickets into the tickets table
-- First, let's create 3 tickets for hailongli@google.com

-- Ticket 1 for hailongli@google.com
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user123', 'Hailong Li', 'hailongli@google.com', 'UA', '1532', 'SFO', 'DEN', '2025-01-01 05:50:00', '2025-01-01 09:23:00');

-- Ticket 2 for hailongli@google.com
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user123', 'Hailong Li', 'hailongli@google.com', 'UA', '1158', 'SFO', 'ORD', '2025-01-01 05:57:00', '2025-01-01 12:13:00');

-- Ticket 3 for hailongli@google.com
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user123', 'Hailong Li', 'hailongli@google.com', 'UA', '300', 'CLE', 'SFO', '2025-01-01 07:15:00', '2025-01-01 09:22:00');

-- Now let's create 17 more tickets for other users
-- Ticket 4
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user456', 'John Smith', 'john.smith@example.com', 'F9', '1057', 'IAH', 'SFO', '2025-01-01 05:52:00', '2025-01-01 08:14:00');

-- Ticket 5
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user789', 'Jane Doe', 'jane.doe@example.com', 'OO', '6403', 'MFR', 'SFO', '2025-01-01 05:57:00', '2025-01-01 07:23:00');

-- Ticket 6
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user101', 'Robert Johnson', 'robert.j@example.com', 'OO', '5628', 'RNO', 'SFO', '2025-01-01 06:19:00', '2025-01-01 07:37:00');

-- Ticket 7
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user202', 'Emily Davis', 'emily.d@example.com', 'CY', '922', 'SFO', 'LAX', '2025-01-01 06:38:00', '2025-01-01 07:49:00');

-- Ticket 8
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user303', 'Michael Wilson', 'michael.w@example.com', 'AA', '338', 'LAX', 'SFO', '2025-01-01 06:44:00', '2025-01-01 08:03:00');

-- Ticket 9
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user404', 'Sarah Brown', 'sarah.b@example.com', 'UA', '1195', 'MCO', 'SFO', '2025-01-01 12:28:00', '2025-01-01 15:06:00');

-- Ticket 10
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user505', 'David Miller', 'david.m@example.com', 'OO', '5632', 'SBA', 'SFO', '2025-01-01 06:56:00', '2025-01-01 08:39:00');

-- Ticket 11
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user606', 'Jennifer Taylor', 'jennifer.t@example.com', 'UA', '1532', 'SFO', 'DEN', '2025-01-02 05:50:00', '2025-01-02 09:23:00');

-- Ticket 12
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user707', 'Thomas Anderson', 'thomas.a@example.com', 'UA', '1158', 'SFO', 'ORD', '2025-01-02 05:57:00', '2025-01-02 12:13:00');

-- Ticket 13
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user808', 'Lisa White', 'lisa.w@example.com', 'F9', '1057', 'IAH', 'SFO', '2025-01-02 05:52:00', '2025-01-02 08:14:00');

-- Ticket 14
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user909', 'Kevin Martin', 'kevin.m@example.com', 'OO', '6403', 'MFR', 'SFO', '2025-01-02 05:57:00', '2025-01-02 07:23:00');

-- Ticket 15
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user1010', 'Amanda Clark', 'amanda.c@example.com', 'OO', '5628', 'RNO', 'SFO', '2025-01-02 06:19:00', '2025-01-02 07:37:00');

-- Ticket 16
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user1111', 'Daniel Lewis', 'daniel.l@example.com', 'CY', '922', 'SFO', 'LAX', '2025-01-02 06:38:00', '2025-01-02 07:49:00');

-- Ticket 17
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user1212', 'Michelle Lee', 'michelle.l@example.com', 'AA', '338', 'LAX', 'SFO', '2025-01-02 06:44:00', '2025-01-02 08:03:00');

-- Ticket 18
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user1313', 'Christopher Harris', 'chris.h@example.com', 'UA', '1195', 'MCO', 'SFO', '2025-01-02 12:28:00', '2025-01-02 15:06:00');

-- Ticket 19
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user1414', 'Jessica Robinson', 'jessica.r@example.com', 'OO', '5632', 'SBA', 'SFO', '2025-01-02 06:56:00', '2025-01-02 08:39:00');

-- Ticket 20
INSERT INTO tickets (user_id, user_name, user_email, airline, flight_number, departure_airport, arrival_airport, departure_time, arrival_time)
VALUES ('user1515', 'Andrew Walker', 'andrew.w@example.com', 'UA', '300', 'CLE', 'SFO', '2025-01-02 07:15:00', '2025-01-02 09:22:00');