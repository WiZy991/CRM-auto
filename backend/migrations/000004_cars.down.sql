DROP TABLE IF EXISTS saved_searches;
DROP TABLE IF EXISTS favorites;
DROP TABLE IF EXISTS car_photos;
DROP TRIGGER IF EXISTS cars_search_vector_trigger ON cars;
DROP FUNCTION IF EXISTS cars_refresh_search_vector();
DROP TABLE IF EXISTS cars;
