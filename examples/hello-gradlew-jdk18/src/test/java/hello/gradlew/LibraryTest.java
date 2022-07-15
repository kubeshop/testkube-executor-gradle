/*
 * This Java source file was generated by the Gradle 'init' task.
 */
package hello.gradlew;

import org.junit.jupiter.api.Test;
import static org.junit.jupiter.api.Assertions.*;

class LibraryTest {
    @Test void someLibraryMethodReturnsTrue() {
        Library classUnderTest = new Library();
        assertTrue(classUnderTest.someLibraryMethod(), "someLibraryMethod should return 'true'");
        System.out.println();
        System.out.printf("JAVA: %s", System.getProperty("java.version"));
        System.out.println();
        assertEquals(System.getProperty("java.version"), "18");
    }
}
